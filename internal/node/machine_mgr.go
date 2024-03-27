package nexnode

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	agentapi "github.com/synadia-io/nex/internal/agent-api"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	EventSubjectPrefix      = "$NEX.events"
	LogSubjectPrefix        = "$NEX.logs"
	WorkloadCacheBucketName = "NEXCACHE"

	defaultHandshakeTimeoutMillis = 5000

	nexTriggerSubject = "x-nex-trigger-subject"
	nexRuntimeNs      = "x-nex-runtime-ns"
)

// The machine manager is responsible for the pool of warm firecracker VMs. This includes starting new
// VMs, stopping VMs, and pulling VMs from the pool on demand
type MachineManager struct {
	closing uint32

	kp   nkeys.KeyPair
	node *Node
	log  *slog.Logger

	allVMs       map[string]*runningFirecracker
	warmVMs      chan *runningFirecracker
	agentClients map[string]*agentapi.AgentClient

	handshakes       map[string]string
	handshakeTimeout time.Duration // TODO: make configurable...

	hostServices *HostServices

	stopMutex map[string]*sync.Mutex
	vmsubz    map[string][]*nats.Subscription

	natsStoreDir string
}

// Initialize a new machine manager instance to manage firecracker VMs
// and private communications between the host and running Nex agents.
func NewMachineManager(node *Node) (*MachineManager, error) {
	// Validate the node config
	if !node.config.Validate() {
		return nil, fmt.Errorf("failed to create new machine manager; invalid node config; %v", node.config.Errors)
	}

	m := &MachineManager{
		node:             node,
		log:              node.log,
		handshakes:       make(map[string]string),
		handshakeTimeout: time.Duration(defaultHandshakeTimeoutMillis * time.Millisecond),

		natsStoreDir: defaultNatsStoreDir,

		allVMs:       make(map[string]*runningFirecracker),
		agentClients: make(map[string]*agentapi.AgentClient),
		warmVMs:      make(chan *runningFirecracker, node.config.MachinePoolSize),

		stopMutex: make(map[string]*sync.Mutex),
		vmsubz:    make(map[string][]*nats.Subscription),
	}

	m.hostServices = NewHostServices(m, node.nc, node.ncint, m.log)
	err := m.hostServices.init()
	if err != nil {
		m.log.Warn("Failed to initialize host services.", slog.Any("err", err))
		return nil, err
	}

	return m, nil
}

// Start the machine manager, maintaining the firecracker VM pool
func (m *MachineManager) Start() {
	m.log.Info("Virtual machine manager starting")

	defer func() {
		if r := recover(); r != nil {
			m.log.Debug(fmt.Sprintf("recovered: %s", r))
		}
	}()

	if !m.node.config.PreserveNetwork {
		err := m.resetCNI()
		if err != nil {
			m.log.Warn("Failed to reset network.", slog.Any("err", err))
		}
	}

	for !m.stopping() {
		select {
		case <-m.node.ctx.Done():
			return
		default:
			if len(m.warmVMs) == m.node.config.MachinePoolSize {
				time.Sleep(runloopSleepInterval)
				continue
			}

			vm, err := createAndStartVM(context.TODO(), m.node.config, m.log)
			if err != nil {
				m.log.Warn("Failed to create VMM for warming pool.", slog.Any("err", err))
				continue
			}

			err = m.setMetadata(vm)
			if err != nil {
				m.log.Warn("Failed to set metadata on VM for warming pool.", slog.Any("err", err))
				continue
			}

			agentClient := agentapi.NewAgentClient(m.node.ncint,
				2*time.Second,
				m.agentHandshakeTimedOut,
				m.agentHandshakeSucceeded,
				m.agentEvent,
				m.agentLog,
				m.log,
			)
			m.agentClients[vm.vmmID] = agentClient
			err = agentClient.Start(vm.vmmID)
			if err != nil {
				m.log.Error("Failed to start agent client", slog.Any("err", err))
				continue
			}

			//go m.awaitHandshake(vm.vmmID)

			m.allVMs[vm.vmmID] = vm
			m.stopMutex[vm.vmmID] = &sync.Mutex{}
			m.node.telemetry.vmCounter.Add(m.node.ctx, 1)

			m.log.Info("Adding new VM to warm pool", slog.Any("ip", vm.ip), slog.String("vmid", vm.vmmID))
			m.warmVMs <- vm // If the pool is full, this line will block until a slot is available.
		}
	}
}

func (m *MachineManager) DeployWorkload(vm *runningFirecracker, request *agentapi.DeployRequest) error {

	status := m.node.ncint.Status()
	m.log.Debug("NATS internal connection status",
		slog.String("vmid", vm.vmmID),
		slog.String("status", status.String()))

	vm.deployRequest = request
	vm.namespace = *request.Namespace
	vm.workloadStarted = time.Now().UTC()

	agentClient := m.agentClients[vm.vmmID]
	deployResponse, err := agentClient.DeployWorkload(request)
	if err != nil {
		return fmt.Errorf("failed to submit request for workload deployment: %s", err)
	}

	if deployResponse.Accepted {
		if request.SupportsTriggerSubjects() {
			for _, tsub := range request.TriggerSubjects {
				sub, err := m.node.nc.Subscribe(tsub, m.generateTriggerHandler(vm, tsub, request))
				if err != nil {
					m.log.Error("Failed to create trigger subject subscription for deployed workload",
						slog.String("vmid", vm.vmmID),
						slog.String("trigger_subject", tsub),
						slog.String("workload_type", *request.WorkloadType),
						slog.Any("err", err),
					)
					_ = m.StopMachine(vm.vmmID, true)
					return err
				}

				m.log.Info("Created trigger subject subscription for deployed workload",
					slog.String("vmid", vm.vmmID),
					slog.String("trigger_subject", tsub),
					slog.String("workload_type", *request.WorkloadType),
				)

				m.vmsubz[vm.vmmID] = append(m.vmsubz[vm.vmmID], sub)
			}
		}
	} else {
		_ = m.StopMachine(vm.vmmID, false)
		return fmt.Errorf("workload rejected by agent: %s", *deployResponse.Message)
	}

	m.node.telemetry.workloadCounter.Add(m.node.ctx, 1, metric.WithAttributes(attribute.String("workload_type", *vm.deployRequest.WorkloadType)))
	m.node.telemetry.workloadCounter.Add(m.node.ctx, 1, metric.WithAttributes(attribute.String("namespace", vm.namespace)), metric.WithAttributes(attribute.String("workload_type", *vm.deployRequest.WorkloadType)))
	m.node.telemetry.deployedByteCounter.Add(m.node.ctx, request.TotalBytes)
	m.node.telemetry.deployedByteCounter.Add(m.node.ctx, request.TotalBytes, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	m.node.telemetry.allocatedVCPUCounter.Add(m.node.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount)
	m.node.telemetry.allocatedVCPUCounter.Add(m.node.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	m.node.telemetry.allocatedMemoryCounter.Add(m.node.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib)
	m.node.telemetry.allocatedMemoryCounter.Add(m.node.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib, metric.WithAttributes(attribute.String("namespace", vm.namespace)))

	return nil
}

// Stops the machine manager, which will in turn stop all firecracker VMs and attempt to clean
// up any applicable resources. Note that all "stopped" events emitted during a stop are best-effort
// and not guaranteed.
func (m *MachineManager) Stop() error {
	if atomic.AddUint32(&m.closing, 1) == 1 {
		m.log.Info("Virtual machine manager stopping")
		close(m.warmVMs)

		for vmID := range m.allVMs {
			err := m.StopMachine(vmID, true)
			if err != nil {
				m.log.Warn("Failed to stop VM", slog.String("vmid", vmID), slog.String("error", err.Error()))
			}
		}

		m.cleanSockets()
	}

	return nil
}

// Stops a single machine, optionally attempting to gracefully undeploy the running workload.
// Will return an error if called with a non-existent workload/vm ID
func (m *MachineManager) StopMachine(vmID string, undeploy bool) error {
	vm, exists := m.allVMs[vmID]
	if !exists {
		return fmt.Errorf("failed to stop machine %s", vmID)
	}

	mutex := m.stopMutex[vmID]
	mutex.Lock()
	defer mutex.Unlock()

	m.log.Debug("Attempting to stop virtual machine", slog.String("vmid", vmID), slog.Bool("undeploy", undeploy))

	for _, sub := range m.vmsubz[vmID] {
		err := sub.Drain()
		if err != nil {
			m.log.Warn(fmt.Sprintf("failed to drain subscription to subject %s associated with vm %s: %s", sub.Subject, vmID, err.Error()))
		}

		m.log.Debug(fmt.Sprintf("drained subscription to subject %s associated with vm %s", sub.Subject, vmID))
	}

	if vm.deployRequest != nil && undeploy {
		agentClient := m.agentClients[vmID]
		err := agentClient.Undeploy()
		if err != nil {
			m.log.Warn("request to undeploy workload via internal NATS connection failed", slog.String("vmid", vm.vmmID), slog.String("error", err.Error()))
			// return err
		}
	}

	vm.shutdown()
	delete(m.allVMs, vmID)
	delete(m.stopMutex, vmID)
	delete(m.vmsubz, vmID)

	_ = m.publishMachineStopped(vm)

	if vm.deployRequest != nil {
		m.node.telemetry.workloadCounter.Add(m.node.ctx, -1, metric.WithAttributes(attribute.String("workload_type", *vm.deployRequest.WorkloadType)))
		m.node.telemetry.workloadCounter.Add(m.node.ctx, -1, metric.WithAttributes(attribute.String("workload_type", *vm.deployRequest.WorkloadType)), metric.WithAttributes(attribute.String("namespace", vm.namespace)))
		m.node.telemetry.deployedByteCounter.Add(m.node.ctx, vm.deployRequest.TotalBytes*-1)
		m.node.telemetry.deployedByteCounter.Add(m.node.ctx, vm.deployRequest.TotalBytes*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	}

	m.node.telemetry.vmCounter.Add(m.node.ctx, -1)
	m.node.telemetry.allocatedVCPUCounter.Add(m.node.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount*-1)
	m.node.telemetry.allocatedVCPUCounter.Add(m.node.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	m.node.telemetry.allocatedMemoryCounter.Add(m.node.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib*-1)
	m.node.telemetry.allocatedMemoryCounter.Add(m.node.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))

	return nil
}

// Looks up a virtual machine by workload/vm ID. Returns nil if machine doesn't exist
func (m *MachineManager) LookupMachine(vmId string) *runningFirecracker {
	vm, exists := m.allVMs[vmId]
	if !exists {
		return nil
	}
	return vm
}

func (m *MachineManager) agentHandshakeTimedOut(agentId string) {
	m.log.Error("Did not receive NATS handshake from agent within timeout.", slog.String("agentId", agentId))
	if len(m.handshakes) == 0 {
		m.log.Error("First handshake failed, shutting down to avoid inconsistent behavior")
		m.node.cancelF()
	}
}

func (m *MachineManager) agentHandshakeSucceeded(agentId string) {
	now := time.Now().UTC()
	m.handshakes[agentId] = now.Format(time.RFC3339)
}

func (m *MachineManager) resetCNI() error {
	m.log.Info("Resetting network")

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
func (m *MachineManager) cleanSockets() {
	dir, err := os.ReadDir(os.TempDir())
	if err != nil {
		m.log.Error("Failed to read temp directory", slog.Any("err", err))
	}

	for _, d := range dir {
		if strings.Contains(d.Name(), fmt.Sprintf(".firecracker.sock-%d-", os.Getpid())) {
			os.Remove(path.Join([]string{"tmp", d.Name()}...))
		}
	}
}

func (m *MachineManager) generateTriggerHandler(vm *runningFirecracker, tsub string, request *agentapi.DeployRequest) func(msg *nats.Msg) {
	return func(msg *nats.Msg) {

		ctx, parentSpan := tracer.Start(
			m.node.ctx,
			"workload-trigger",
			trace.WithNewRoot(),
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("name", *request.WorkloadName),
				attribute.String("namespace", vm.namespace),
				attribute.String("trigger-subject", msg.Subject),
			))

		defer parentSpan.End()

		intmsg := nats.NewMsg(fmt.Sprintf("agentint.%s.trigger", vm.vmmID))
		// TODO: inject tracer context into message header
		intmsg.Data = msg.Data

		intmsg.Header.Add(nexTriggerSubject, msg.Subject)

		cctx, childSpan := tracer.Start(
			ctx,
			"internal request",
			trace.WithSpanKind(trace.SpanKindClient),
		)

		otel.GetTextMapPropagator().Inject(cctx, propagation.HeaderCarrier(msg.Header))

		// TODO: make the agent's exec handler extract and forward the otel context
		// so it continues in the host services like kv, obj, msg, etc
		resp, err := m.node.ncint.RequestMsg(intmsg, time.Millisecond*10000) // FIXME-- make timeout configurable
		childSpan.End()

		//for reference - this is what agent exec would also do
		//ctx = otel.GetTextMapPropagator().Extract(cctx, propagation.HeaderCarrier(msg.Header))

		parentSpan.AddEvent("Completed internal request")
		if err != nil {
			parentSpan.SetStatus(codes.Error, "Internal trigger request failed")
			parentSpan.RecordError(err)
			m.log.Error("Failed to request agent execution via internal trigger subject",
				slog.Any("err", err),
				slog.String("trigger_subject", tsub),
				slog.String("workload_type", *request.WorkloadType),
				slog.String("vmid", vm.vmmID),
			)

			m.node.telemetry.functionFailedTriggers.Add(m.node.ctx, 1)
			m.node.telemetry.functionFailedTriggers.Add(m.node.ctx, 1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
			m.node.telemetry.functionFailedTriggers.Add(m.node.ctx, 1, metric.WithAttributes(attribute.String("workload_name", *vm.deployRequest.WorkloadName)))
			_ = m.publishFunctionExecFailed(vm, *request.WorkloadName, tsub, err)
		} else if resp != nil {
			parentSpan.SetStatus(codes.Ok, "Trigger succeeded")
			runtimeNs := resp.Header.Get(nexRuntimeNs)
			m.log.Debug("Received response from execution via trigger subject",
				slog.String("vmid", vm.vmmID),
				slog.String("trigger_subject", tsub),
				slog.String("workload_type", *request.WorkloadType),
				slog.String("function_run_time_nanosec", runtimeNs),
				slog.Int("payload_size", len(resp.Data)),
			)

			runTimeNs64, err := strconv.ParseInt(runtimeNs, 10, 64)
			if err != nil {
				m.log.Warn("failed to log function runtime", slog.Any("err", err))
			}
			_ = m.publishFunctionExecSucceeded(vm, tsub, runTimeNs64)
			parentSpan.AddEvent("published success event")

			m.node.telemetry.functionTriggers.Add(m.node.ctx, 1)
			m.node.telemetry.functionTriggers.Add(m.node.ctx, 1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
			m.node.telemetry.functionTriggers.Add(m.node.ctx, 1, metric.WithAttributes(attribute.String("workload_name", *vm.deployRequest.WorkloadName)))
			m.node.telemetry.functionRunTimeNano.Add(m.node.ctx, runTimeNs64)
			m.node.telemetry.functionRunTimeNano.Add(m.node.ctx, runTimeNs64, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
			m.node.telemetry.functionRunTimeNano.Add(m.node.ctx, runTimeNs64, metric.WithAttributes(attribute.String("workload_name", *vm.deployRequest.WorkloadName)))

			err = msg.Respond(resp.Data)
			//_ = tracerProvider.ForceFlush(ctx)
			if err != nil {
				parentSpan.SetStatus(codes.Error, "Failed to respond to trigger subject")
				parentSpan.RecordError(err)
				m.log.Error("Failed to respond to trigger subject subscription request for deployed workload",
					slog.String("vmid", vm.vmmID),
					slog.String("trigger_subject", tsub),
					slog.String("workload_type", *request.WorkloadType),
					slog.Any("err", err),
				)
			}
		}
	}
}

func (m *MachineManager) setMetadata(vm *runningFirecracker) error {
	return vm.setMetadata(&agentapi.MachineMetadata{
		Message:      agentapi.StringOrNil("Host-supplied metadata"),
		NodeNatsHost: vm.config.InternalNodeHost,
		NodeNatsPort: vm.config.InternalNodePort,
		VmID:         &vm.vmmID,
	})
}

func (m *MachineManager) stopping() bool {
	return (atomic.LoadUint32(&m.closing) > 0)
}
