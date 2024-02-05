package nexnode

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/rs/xid"

	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

// Represents an instance of a single firecracker VM containing the nex agent.
type runningFirecracker struct {
	vmmCtx    context.Context
	vmmCancel context.CancelFunc
	vmmID     string

	closing        uint32
	deployRequest  *agentapi.DeployRequest
	ip             net.IP
	log            *slog.Logger
	machine        *firecracker.Machine
	machineStarted time.Time
	// mutex           sync.Mutex
	namespace       string
	workloadStarted time.Time
}

func (vm *runningFirecracker) shutdown() {
	if atomic.AddUint32(&vm.closing, 1) == 1 {
		vm.log.Info("Machine stopping",
			slog.String("vmid", vm.vmmID),
			slog.String("ip", vm.ip.String()),
		)

		err := vm.machine.StopVMM()
		if err != nil {
			vm.log.Error("Failed to stop firecracker VM", slog.Any("err", err))
		}

		err = os.Remove(vm.machine.Cfg.SocketPath)
		if err != nil {
			if !errors.Is(err, fs.ErrExist) {
				vm.log.Warn("Failed to delete firecracker socket", slog.Any("err", err))
			}
		}

		// NOTE: we're not deleting the firecracker machine logs ... they're in a tempfs so they'll eventually
		// go away but we might want them kept around for troubleshooting

		rootFs := getRootFsPath(vm.vmmID)
		err = os.Remove(rootFs)
		if err != nil {
			if !errors.Is(err, fs.ErrExist) {
				vm.log.Warn("Failed to delete firecracker rootfs", slog.Any("err", err))
			}
		}
	}
}

// Create a VMM with a given set of options and start the VM
func createAndStartVM(ctx context.Context, config *NodeConfiguration, log *slog.Logger) (*runningFirecracker, error) {
	vmmID := xid.New().String()

	fcCfg, err := generateFirecrackerConfig(vmmID, config)
	if err != nil {
		log.Error("Failed to generate firecracker configuration", slog.Any("config", config))
		return nil, err
	}

	err = copy(config.RootFsFilepath, *fcCfg.Drives[0].PathOnHost)

	if err != nil {
		log.Error("Failed to copy rootfs to temp location", slog.Any("err", err))
		return nil, err
	}

	// TODO: can we please not use logrus here amazon?
	machineOpts := []firecracker.Opt{
		firecracker.WithLogger(log.With(slog.Bool("firecracker", true), slog.String("vmmid", vmmID))),
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
		VmId:         &vmmID,
		NodeNatsHost: config.InternalNodeHost,
		NodeNatsPort: config.InternalNodePort,
		Message:      agentapi.StringOrNil("Host-supplied metadata"),
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

	log.Info("Machine started",
		slog.String("vmid", vmmID),
		slog.Any("ip", ip),
		slog.Any("gateway", gw),
		slog.String("netmask", mask.String()),
		slog.String("hosttap", hosttap),
		slog.String("nats_host", *md.NodeNatsHost),
		slog.Int("nats_port", *md.NodeNatsPort),
	)

	return &runningFirecracker{
		log:            log,
		machine:        m,
		ip:             ip,
		machineStarted: time.Now().UTC(),
		vmmCancel:      vmmCancel,
		vmmCtx:         vmmCtx,
		vmmID:          vmmID,
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
		ForwardSignals:  make([]os.Signal, 0),
		KernelImagePath: config.KernelFilepath,
		LogPath:         fmt.Sprintf("%s.log", socket),
		NetworkInterfaces: []firecracker.NetworkInterface{{
			AllowMMDS: true,
			// Use CNI to get dynamic IP
			CNIConfiguration: &firecracker.CNIConfiguration{
				IfName:      *config.CNI.InterfaceName,
				NetworkName: *config.CNI.NetworkName,
			},
			//OutRateLimiter: firecracker.NewRateLimiter(..., ...),
			//InRateLimiter: firecracker.NewRateLimiter(..., ...),
		}},
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(int64(*config.MachineTemplate.VcpuCount)),
			MemSizeMib: firecracker.Int64(int64(*config.MachineTemplate.MemSizeMib)),
		},
		MmdsVersion: firecracker.MMDSv2,
		SocketPath:  socket,
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

