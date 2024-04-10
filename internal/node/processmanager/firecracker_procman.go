package processmanager

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"github.com/synadia-io/nex/internal/models"
	"github.com/synadia-io/nex/internal/node/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const runloopSleepInterval = 100 * time.Millisecond

type FirecrackerProcessManager struct {
	closing   uint32
	config    *models.NodeConfiguration
	ctx       context.Context
	log       *slog.Logger
	stopMutex map[string]*sync.Mutex
	t         *observability.Telemetry

	allVMs  map[string]*runningFirecracker
	warmVMs chan *runningFirecracker

	delegate       ProcessDelegate
	deployRequests map[string]*agentapi.DeployRequest
}

func NewFirecrackerProcessManager(
	log *slog.Logger,
	config *models.NodeConfiguration,
	telemetry *observability.Telemetry,
	ctx context.Context,
) (*FirecrackerProcessManager, error) {
	return &FirecrackerProcessManager{
		config: config,
		t:      telemetry,
		log:    log,
		ctx:    ctx,

		allVMs:         make(map[string]*runningFirecracker),
		warmVMs:        make(chan *runningFirecracker, config.MachinePoolSize),
		stopMutex:      make(map[string]*sync.Mutex),
		deployRequests: make(map[string]*agentapi.DeployRequest),
	}, nil
}

func (f *FirecrackerProcessManager) ListProcesses() ([]ProcessInfo, error) {
	pinfos := make([]ProcessInfo, 0)

	for workloadId, vm := range f.allVMs {
		// Ignore "pending" processes that don't have workloads on them yet
		if vm.deployRequest != nil {
			pinfo := ProcessInfo{
				ID:            workloadId,
				Name:          *vm.deployRequest.WorkloadName,
				Namespace:     *vm.deployRequest.Namespace,
				DeployRequest: vm.deployRequest,
			}
			pinfos = append(pinfos, pinfo)
		}
	}

	return pinfos, nil
}

// Preparing a workload reads from the warmVMs channel
func (f *FirecrackerProcessManager) PrepareWorkload(workloadId string, deployRequest *agentapi.DeployRequest) error {
	vm := <-f.warmVMs
	if vm == nil {
		return fmt.Errorf("could not prepare workload, no available firecracker VM")
	}

	vm.deployRequest = deployRequest
	vm.namespace = *deployRequest.Namespace
	vm.workloadStarted = time.Now().UTC()

	f.deployRequests[vm.vmmID] = deployRequest

	f.t.AllocatedVCPUCounter.Add(f.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount)
	f.t.AllocatedVCPUCounter.Add(f.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	f.t.AllocatedMemoryCounter.Add(f.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib)
	f.t.AllocatedMemoryCounter.Add(f.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib, metric.WithAttributes(attribute.String("namespace", vm.namespace)))

	return nil
}

func (f *FirecrackerProcessManager) Stop() error {
	if atomic.AddUint32(&f.closing, 1) == 1 {
		f.log.Info("Firecracker process manager stopping")
		close(f.warmVMs)

		for vmID := range f.allVMs {
			err := f.StopProcess(vmID)
			if err != nil {
				f.log.Warn("Failed to stop firecracker process", slog.String("workload_id", vmID), slog.String("error", err.Error()))
			}
		}

		f.cleanSockets()
	}

	return nil
}

func (f *FirecrackerProcessManager) Start(delegate ProcessDelegate) error {
	f.log.Info("Firecracker VM process manager starting")
	f.delegate = delegate

	defer func() {
		if r := recover(); r != nil {
			f.log.Debug(fmt.Sprintf("recovered: %s", r))
		}
	}()

	if !f.config.PreserveNetwork {
		err := f.resetCNI()
		if err != nil {
			f.log.Warn("Failed to reset network.", slog.Any("err", err))
		}
	}

	for !f.stopping() {
		select {
		case <-f.ctx.Done():
			return nil
		default:
			if len(f.warmVMs) == f.config.MachinePoolSize {
				time.Sleep(runloopSleepInterval)
				continue
			}

			vm, err := createAndStartVM(context.TODO(), f.config, f.log)
			if err != nil {
				f.log.Warn("Failed to create VMM for warming pool.", slog.Any("err", err))
				continue
			}

			err = f.setMetadata(vm)
			if err != nil {
				f.log.Warn("Failed to set metadata on VM for warming pool.", slog.Any("err", err))
				continue
			}

			f.allVMs[vm.vmmID] = vm
			f.stopMutex[vm.vmmID] = &sync.Mutex{}

			f.t.VmCounter.Add(f.ctx, 1)

			go f.delegate.OnProcessStarted(vm.vmmID)

			f.log.Info("Adding new VM to warm pool", slog.Any("ip", vm.ip), slog.String("vmid", vm.vmmID))
			f.warmVMs <- vm // If the pool is full, this line will block until a slot is available.
		}
	}

	return nil
}

func (f *FirecrackerProcessManager) StopProcess(workloadID string) error {
	vm, exists := f.allVMs[workloadID]
	if !exists {
		return fmt.Errorf("failed to stop machine %s", workloadID)
	}

	delete(f.deployRequests, workloadID)

	mutex := f.stopMutex[workloadID]
	mutex.Lock()
	defer mutex.Unlock()

	f.log.Debug("Attempting to stop virtual machine", slog.String("workload_id", workloadID))

	vm.shutdown()
	delete(f.allVMs, workloadID)
	delete(f.stopMutex, workloadID)

	if vm.deployRequest != nil {
		f.t.WorkloadCounter.Add(f.ctx, -1, metric.WithAttributes(attribute.String("workload_type", *vm.deployRequest.WorkloadType)))
		f.t.WorkloadCounter.Add(f.ctx, -1, metric.WithAttributes(attribute.String("workload_type", *vm.deployRequest.WorkloadType)), metric.WithAttributes(attribute.String("namespace", vm.namespace)))
		f.t.DeployedByteCounter.Add(f.ctx, vm.deployRequest.TotalBytes*-1)
		f.t.DeployedByteCounter.Add(f.ctx, vm.deployRequest.TotalBytes*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	}

	f.t.VmCounter.Add(f.ctx, -1)
	f.t.AllocatedVCPUCounter.Add(f.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount*-1)
	f.t.AllocatedVCPUCounter.Add(f.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	f.t.AllocatedMemoryCounter.Add(f.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib*-1)
	f.t.AllocatedMemoryCounter.Add(f.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))

	return nil
}

func (f *FirecrackerProcessManager) Lookup(workloadID string) (*agentapi.DeployRequest, error) {
	if request, ok := f.deployRequests[workloadID]; ok {
		return request, nil
	}

	// Per contract, a non-prepared workload returns nil, not error
	return nil, nil
}

func (f *FirecrackerProcessManager) resetCNI() error {
	f.log.Info("Resetting network")

	err := os.RemoveAll("/var/lib/cni")
	if err != nil {
		return err
	}

	err = os.Mkdir("/var/lib/cni", 0755)
	if err != nil {
		return err
	}

	cmd := exec.Command("bash", "-c", "for name in $(ifconfig -a | sed 's/[ \t].*//;/^\\(lo\\|\\)$/d' | grep veth); do ip link delete $name; done")
	err = cmd.Start()
	if err != nil {
		return err
	}
	err = cmd.Wait()
	if err != nil {
		return err
	}

	return nil
}

// Remove firecracker VM sockets created by this pid
func (f *FirecrackerProcessManager) cleanSockets() {
	dir, err := os.ReadDir(os.TempDir())
	if err != nil {
		f.log.Error("Failed to read temp directory", slog.Any("err", err))
	}

	for _, d := range dir {
		if strings.Contains(d.Name(), fmt.Sprintf(".firecracker.sock-%d-", os.Getpid())) {
			os.Remove(path.Join([]string{"tmp", d.Name()}...))
		}
	}
}

func (f *FirecrackerProcessManager) setMetadata(vm *runningFirecracker) error {
	return vm.setMetadata(&agentapi.MachineMetadata{
		Message:      agentapi.StringOrNil("Host-supplied metadata"),
		NodeNatsHost: vm.config.InternalNodeHost,
		NodeNatsPort: vm.config.InternalNodePort,
		VmID:         &vm.vmmID,
	})
}

func (f *FirecrackerProcessManager) stopping() bool {
	return (atomic.LoadUint32(&f.closing) > 0)
}
