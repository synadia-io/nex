package nexnode

// Functions dealing specifically with the firecracker SDK

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/rs/xid"
	log "github.com/sirupsen/logrus"
)

// Create a VMM with a given set of options and start the VM
func createAndStartVM(ctx context.Context, config *NodeConfiguration) (*runningFirecracker, error) {
	vmmID := xid.New().String()

	copy(config.RootFsPath, "/tmp/rootfs-"+vmmID+".ext4")

	fcCfg, err := generateFirecrackerConfig(vmmID, config)
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
			// WithStdin(os.Stdin).
			// WithStdout(os.Stdout).
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

	if err := m.Start(vmmCtx); err != nil {
		vmmCancel()
		return nil, fmt.Errorf("failed to start machine: %v", err)
	}

	log.WithField("ip", m.Cfg.NetworkInterfaces[0].StaticConfiguration.IPConfiguration.IPAddr.IP).Info("machine started")

	ip := m.Cfg.NetworkInterfaces[0].StaticConfiguration.IPConfiguration.IPAddr.IP
	return &runningFirecracker{
		vmmCtx:         vmmCtx,
		vmmCancel:      vmmCancel,
		vmmID:          vmmID,
		machine:        m,
		ip:             ip,
		machineStarted: time.Now().UTC(),
		agentClient:    agentapi.NewAgentClient(fmt.Sprintf("%s:8081", ip.String())),
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

	return firecracker.Config{
		SocketPath:      socket,
		KernelImagePath: config.KernelPath,
		LogPath:         fmt.Sprintf("%s.log", socket),
		Drives: []models.Drive{{
			DriveID:      firecracker.String("1"),
			PathOnHost:   firecracker.String("/tmp/rootfs-" + id + ".ext4"),
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
