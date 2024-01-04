package nexnode

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	controlapi "github.com/ConnectEverything/nex/control-api"
	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/rs/xid"
	log "github.com/sirupsen/logrus"
)

// Represents an instance of a single firecracker VM containing the nex agent.
type runningFirecracker struct {
	vmmCtx                context.Context
	vmmCancel             context.CancelFunc
	vmmID                 string
	machine               *firecracker.Machine
	ip                    net.IP
	workloadStarted       time.Time
	machineStarted        time.Time
	workloadSpecification controlapi.RunRequest
	namespace             string
}

func (vm *runningFirecracker) shutDown(forensic bool) {

	rootFs := getRootFsPath(vm.vmmID)

	log.WithField("vmid", vm.vmmID).
		WithField("ip", vm.ip).
		WithField("rootfs", rootFs).
		Info("Machine stopping")

	err := vm.machine.StopVMM()
	if err != nil {
		log.WithError(err).Error("Failed to stop firecracker VM")
	}
	if !forensic {
		err = os.Remove(vm.machine.Cfg.SocketPath)
		if err != nil {
			if !errors.Is(err, fs.ErrExist) {
				log.WithError(err).Warn("Failed to delete firecracker socket")
			}
		}

		// NOTE: we're not deleting the firecracker machine logs ... they're in a tempfs so they'll eventually
		// go away but we might want them kept around for troubleshooting

		err = os.Remove(rootFs)
		if err != nil {
			if !errors.Is(err, fs.ErrExist) {
				log.WithError(err).Warn("Failed to delete firecracker rootfs")
			}
		}
	}
}

// Create a VMM with a given set of options and start the VM
func createAndStartVM(ctx context.Context, config *NodeConfiguration) (*runningFirecracker, error) {
	vmmID := xid.New().String()

	fcCfg, err := generateFirecrackerConfig(vmmID, config)
	if err != nil {
		log.WithError(err).Error("Failed to generate firecracker configuration")
		return nil, err
	}
	err = copy(config.RootFsPath, *fcCfg.Drives[0].PathOnHost)
	if err != nil {
		log.WithError(err).Error("Failed to copy rootfs to temp location")
		return nil, err
	}

	if err != nil {
		log.Errorf("Error: %s", err)
		return nil, err
	}
	logger := log.New()

	machineOpts := []firecracker.Opt{
		firecracker.WithLogger(log.NewEntry(logger)),
	}

	firecrackerBinary, err := exec.LookPath("firecracker")
	if err != nil {
		return nil, err
	}

	finfo, err := os.Stat(firecrackerBinary)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("binary %q does not exist: %v", firecrackerBinary, err)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to stat binary, %q: %v", firecrackerBinary, err)
	}

	if finfo.IsDir() {
		return nil, fmt.Errorf("binary, %q, is a directory", firecrackerBinary)
	} else if finfo.Mode()&0111 == 0 {
		return nil, fmt.Errorf("binary, %q, is not executable. Check permissions of binary", firecrackerBinary)
	}

	if fcCfg.JailerCfg == nil {
		cmd := firecracker.VMCommandBuilder{}.
			WithBin(firecrackerBinary).
			WithSocketPath(fcCfg.SocketPath).
			WithStderr(os.Stderr).
			Build(ctx)

		machineOpts = append(machineOpts, firecracker.WithProcessRunner(cmd))
	}

	vmmCtx, vmmCancel := context.WithCancel(ctx)

	m, err := firecracker.NewMachine(vmmCtx, fcCfg, machineOpts...)
	if err != nil {
		vmmCancel()
		return nil, fmt.Errorf("failed creating machine: %s", err)
	}
	md := agentapi.MachineMetadata{
		VmId:            vmmID,
		NodeNatsAddress: config.InternalNodeHost,
		NodePort:        config.InternalNodePort,
		Message:         "Host-supplied metadata",
	}

	if err := m.Start(vmmCtx); err != nil {
		vmmCancel()
		return nil, fmt.Errorf("failed to start machine: %v", err)
	}
	err = m.SetMetadata(vmmCtx, md)
	if err != nil {
		vmmCancel()
		return nil, fmt.Errorf("failed to set machine metadata: %s", err)
	}

	gw := m.Cfg.NetworkInterfaces[0].StaticConfiguration.IPConfiguration.Gateway
	ip := m.Cfg.NetworkInterfaces[0].StaticConfiguration.IPConfiguration.IPAddr.IP
	hosttap := m.Cfg.NetworkInterfaces[0].StaticConfiguration.HostDevName
	mask := m.Cfg.NetworkInterfaces[0].StaticConfiguration.IPConfiguration.IPAddr.Mask
	log.WithField("ip", ip).WithField("gateway", gw).WithField("netmask", mask).WithField("hosttap", hosttap).Info("Machine started")

	return &runningFirecracker{
		vmmCtx:         vmmCtx,
		vmmCancel:      vmmCancel,
		vmmID:          vmmID,
		machine:        m,
		ip:             ip,
		machineStarted: time.Now().UTC(),
	}, nil
}

func copy(src string, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	err = os.WriteFile(dst, data, 0644)
	return err
}

func generateFirecrackerConfig(id string, config *NodeConfiguration) (firecracker.Config, error) {
	socket := getSocketPath(id)
	rootPath := getRootFsPath(id)

	return firecracker.Config{
		SocketPath:      socket,
		KernelImagePath: config.KernelPath,
		LogPath:         fmt.Sprintf("%s.log", socket),
		Drives: []models.Drive{{
			DriveID:      firecracker.String("1"),
			PathOnHost:   &rootPath,
			IsRootDevice: firecracker.Bool(true),
			IsReadOnly:   firecracker.Bool(false),
			// RateLimiter: firecracker.NewRateLimiter(
			// 	// bytes/s
			// 	models.TokenBucket{
			// 		OneTimeBurst: firecracker.Int64(1024 * 1024), // 1 MiB/s
			// 		RefillTime:   firecracker.Int64(500),         // 0.5s
			// 		Size:         firecracker.Int64(1024 * 1024),
			// 	},
			// 	// ops/s
			// 	models.TokenBucket{
			// 		OneTimeBurst: firecracker.Int64(100),  // 100 iops
			// 		RefillTime:   firecracker.Int64(1000), // 1s
			// 		Size:         firecracker.Int64(100),
			// 	}),
		}},
		NetworkInterfaces: []firecracker.NetworkInterface{{
			AllowMMDS: true,
			// Use CNI to get dynamic IP
			CNIConfiguration: &firecracker.CNIConfiguration{
				NetworkName: config.CNI.NetworkName,
				IfName:      config.CNI.InterfaceName,
			},
			//OutRateLimiter: firecracker.NewRateLimiter(..., ...),
			//InRateLimiter: firecracker.NewRateLimiter(..., ...),
		}},
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(int64(config.MachineTemplate.VcpuCount)),
			MemSizeMib: firecracker.Int64(int64(config.MachineTemplate.MemSizeMib)),
		},
		MmdsVersion: firecracker.MMDSv2,
	}, nil
}

func getRootFsPath(vmmID string) string {
	filename := fmt.Sprintf("rootfs-%s.ext4", vmmID)
	dir := os.TempDir()

	return filepath.Join(dir, filename)
}

func getSocketPath(vmmID string) string {
	filename := strings.Join([]string{
		".firecracker.sock",
		strconv.Itoa(os.Getpid()),
		vmmID,
	},
		"-",
	)
	dir := os.TempDir()

	return filepath.Join(dir, filename)
}
