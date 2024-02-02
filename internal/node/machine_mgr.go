package nexnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
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
	closing    uint32
	config     *NodeConfiguration
	kp         nkeys.KeyPair
	log        *slog.Logger
	nc         *nats.Conn
	ncInternal *nats.Conn
	ctx        context.Context
	t          *Telemetry

	allVms  map[string]*runningFirecracker
	warmVms chan *runningFirecracker

	handshakes       map[string]string
	handshakeTimeout time.Duration // TODO: make configurable...

	natsStoreDir string
	publicKey    string
}

// Initialize a new machine manager instance to manage firecracker VMs
// and private communications between the host and running Nex agents.
func NewMachineManager(
	ctx context.Context,
	nodeKeypair nkeys.KeyPair,
	publicKey string,
	nc, ncint *nats.Conn,
	config *NodeConfiguration,
	log *slog.Logger,
	telemetry *Telemetry,
) (*MachineManager, error) {
	// Validate the node config
	if !config.Validate() {
		return nil, fmt.Errorf("failed to create new machine manager; invalid node config; %v", config.Errors)
	}

	m := &MachineManager{
		config:           config,
		handshakes:       make(map[string]string),
		handshakeTimeout: time.Duration(defaultHandshakeTimeoutMillis * time.Millisecond),
		kp:               nodeKeypair,
		log:              log,
		natsStoreDir:     defaultNatsStoreDir,
		nc:               nc,
		ncInternal:       ncint,
		publicKey:        publicKey,
		t:                telemetry,
		ctx:              ctx,

		allVms:  make(map[string]*runningFirecracker),
		warmVms: make(chan *runningFirecracker, config.MachinePoolSize-1),
	}

	_, err := m.ncInternal.Subscribe("agentint.handshake", m.handleHandshake)
	if err != nil {
		return nil, err
	}

	_, err = m.ncInternal.Subscribe("agentint.*.events.*", m.handleAgentEvent)
	if err != nil {
		return nil, err
	}

	_, err = m.ncInternal.Subscribe("agentint.*.logs", m.handleAgentLog)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// Starts the machine manager. Publishes a node started event and starts the
// goroutine responsible for keeping the firecracker VM pool full
func (m *MachineManager) Start() error {
	m.log.Info("Virtual machine manager starting")

	go m.fillPool()
	_ = m.PublishNodeStarted()

	return nil
}

func (m *MachineManager) DeployWorkload(vm *runningFirecracker, request *agentapi.DeployRequest) error {
	bytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	status := m.ncInternal.Status()
	m.log.Debug("NATS internal connection status",
		slog.String("vmid", vm.vmmID),
		slog.String("status", status.String()))

	subject := fmt.Sprintf("agentint.%s.deploy", vm.vmmID)
	resp, err := m.ncInternal.Request(subject, bytes, 1*time.Second)
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return errors.New("timed out waiting for acknowledgement of workload deployment")
		} else {
			return fmt.Errorf("failed to submit request for workload deployment: %s", err)
		}
	}

	var deployResponse agentapi.DeployResponse
	err = json.Unmarshal(resp.Data, &deployResponse)
	if err != nil {
		return err
	}

	if !deployResponse.Accepted {
		return fmt.Errorf("workload rejected by agent: %s", *deployResponse.Message)
	} else if request.SupportsTriggerSubjects() {
		for _, tsub := range request.TriggerSubjects {
			_, err := m.nc.Subscribe(tsub, func(msg *nats.Msg) {
				intmsg := nats.NewMsg(fmt.Sprintf("agentint.%s.trigger", vm.vmmID))
				intmsg.Data = msg.Data
				intmsg.Header.Add(nexTriggerSubject, msg.Subject)

				resp, err := m.ncInternal.RequestMsg(intmsg, time.Millisecond*10000) // FIXME-- make timeout configurable
				if err != nil {
					m.log.Error("Failed to request agent execution via internal trigger subject",
						slog.Any("err", err),
						slog.String("trigger_subject", tsub),
						slog.String("workload_type", *request.WorkloadType),
						slog.String("vmid", vm.vmmID),
					)

					m.t.functionFailedTriggers.Add(m.ctx, 1)
					m.t.functionFailedTriggers.Add(m.ctx, 1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
					m.t.functionFailedTriggers.Add(m.ctx, 1, metric.WithAttributes(attribute.String("workload_name", *vm.deployRequest.WorkloadName)))
				} else if resp != nil {
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

					m.t.functionTriggers.Add(m.ctx, 1)
					m.t.functionTriggers.Add(m.ctx, 1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
					m.t.functionTriggers.Add(m.ctx, 1, metric.WithAttributes(attribute.String("workload_name", *vm.deployRequest.WorkloadName)))
					m.t.functionRunTimeNano.Add(m.ctx, runTimeNs64)
					m.t.functionRunTimeNano.Add(m.ctx, runTimeNs64, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
					m.t.functionRunTimeNano.Add(m.ctx, runTimeNs64, metric.WithAttributes(attribute.String("workload_name", *vm.deployRequest.WorkloadName)))

					if len(resp.Data) > 0 {
						err = msg.Respond(resp.Data)
						if err != nil {
							m.log.Error("Failed to respond to trigger subject subscription request for deployed workload",
								slog.String("vmid", vm.vmmID),
								slog.String("trigger_subject", tsub),
								slog.String("workload_type", *request.WorkloadType),
								slog.Any("err", err),
							)
						}
					}
				}
			})
			if err != nil {
				m.log.Error("Failed to create trigger subject subscription for deployed workload",
					slog.String("vmid", vm.vmmID),
					slog.String("trigger_subject", tsub),
					slog.String("workload_type", *request.WorkloadType),
					slog.Any("err", err),
				)
				// TODO-- rollback the otherwise accepted deployment and return the error below...
				// return err
			}

			m.log.Info("Created trigger subject subscription for deployed workload",
				slog.String("vmid", vm.vmmID),
				slog.String("trigger_subject", tsub),
				slog.String("workload_type", *request.WorkloadType),
			)
		}
	}

	vm.workloadStarted = time.Now().UTC()
	vm.namespace = *request.Namespace
	vm.deployRequest = request

	m.t.workloadCounter.Add(m.ctx, 1, metric.WithAttributes(attribute.String("workload_type", *vm.deployRequest.WorkloadType)))
	m.t.workloadCounter.Add(m.ctx, 1, metric.WithAttributes(attribute.String("namespace", vm.namespace)), metric.WithAttributes(attribute.String("workload_type", *vm.deployRequest.WorkloadType)))
	m.t.deployedByteCounter.Add(m.ctx, request.TotalBytes)
	m.t.deployedByteCounter.Add(m.ctx, request.TotalBytes, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	m.t.allocatedVCPUCounter.Add(m.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount)
	m.t.allocatedVCPUCounter.Add(m.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	m.t.allocatedMemoryCounter.Add(m.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib)
	m.t.allocatedMemoryCounter.Add(m.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib, metric.WithAttributes(attribute.String("namespace", vm.namespace)))

	return nil
}

// Stops the machine manager, which will in turn stop all firecracker VMs and attempt to clean
// up any applicable resources. Note that all "stopped" events emitted during a stop are best-effort
// and not guaranteed.
func (m *MachineManager) Stop() error {
	if atomic.AddUint32(&m.closing, 1) == 1 {
		m.log.Info("Virtual machine manager stopping")
		close(m.warmVms)

		for _, vm := range m.allVms {
			vm.shutdown(m.log)
			_ = m.PublishMachineStopped(vm)
		}

		// Now empty the leftovers in the pool
		for vm := range m.warmVms {
			vm.shutdown(m.log)

			if vm.deployRequest != nil {
				m.t.workloadCounter.Add(m.ctx, -1, metric.WithAttributes(attribute.String("workload_type", *vm.deployRequest.WorkloadType)))
				m.t.workloadCounter.Add(m.ctx, -1, metric.WithAttributes(attribute.String("workload_type", *vm.deployRequest.WorkloadType)), metric.WithAttributes(attribute.String("namespace", vm.namespace)))
				m.t.deployedByteCounter.Add(m.ctx, vm.deployRequest.TotalBytes*-1)
				m.t.deployedByteCounter.Add(m.ctx, vm.deployRequest.TotalBytes*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
			}

			m.t.vmCounter.Add(m.ctx, -1)
			m.t.allocatedVCPUCounter.Add(m.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount*-1)
			m.t.allocatedVCPUCounter.Add(m.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
			m.t.allocatedMemoryCounter.Add(m.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib*-1)
			m.t.allocatedMemoryCounter.Add(m.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
		}

		_ = m.nc.Drain()
		m.cleanSockets()

		_ = m.PublishNodeStopped()
	}

	return nil
}

// Stops a single machine. Will return an error if called with a non-existent workload/vm ID
func (m *MachineManager) StopMachine(vmId string) error {
	vm, exists := m.allVms[vmId]
	if !exists {
		return errors.New("no such VM")
	}

	subject := fmt.Sprintf("agentint.%s.undeploy", vm.vmmID)
	// we do a request here so we don't tell firecracker to shut down until
	// we get a reply
	_, err := m.ncInternal.Request(subject, []byte{}, 500*time.Millisecond)
	if err != nil {
		return err
	}

	vm.shutdown(m.log)
	delete(m.allVms, vmId)

	_ = m.PublishMachineStopped(vm)

	if vm.deployRequest != nil {
		m.t.workloadCounter.Add(m.ctx, -1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
		m.t.deployedByteCounter.Add(m.ctx, vm.deployRequest.TotalBytes*-1)
		m.t.deployedByteCounter.Add(m.ctx, vm.deployRequest.TotalBytes*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	}

	m.t.vmCounter.Add(m.ctx, -1)
	m.t.allocatedVCPUCounter.Add(m.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount*-1)
	m.t.allocatedVCPUCounter.Add(m.ctx, *vm.machine.Cfg.MachineCfg.VcpuCount*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	m.t.allocatedMemoryCounter.Add(m.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib*-1)
	m.t.allocatedMemoryCounter.Add(m.ctx, *vm.machine.Cfg.MachineCfg.MemSizeMib*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))

	return nil
}

// Looks up a virtual machine by workload/vm ID. Returns nil if machine doesn't exist
func (m *MachineManager) LookupMachine(vmId string) *runningFirecracker {
	vm, exists := m.allVms[vmId]
	if !exists {
		return nil
	}
	return vm
}

// Called by a consumer looking to submit a workload into a virtual machine. Prior to that, the machine
// must be taken out of the pool. Taking a machine out of the pool unblocks a goroutine that will automatically
// replenish
func (m *MachineManager) TakeFromPool() (*runningFirecracker, error) {
	running := <-m.warmVms
	//m.allVms[running.vmmID] = running
	return running, nil
}

func (m *MachineManager) fillPool() {
	defer func() {
		if r := recover(); r != nil {
			m.log.Info(fmt.Sprintf("recovered: %s", r))
		}
	}()

	for !m.stopping() {
		select {
		case <-m.ctx.Done():
			return
		default:
			vm, err := createAndStartVM(m.ctx, m.config, m.log)
			if err != nil {
				m.log.Warn("Failed to create VMM for warming pool.", slog.Any("err", err))
				continue
			}

			go m.awaitHandshake(vm.vmmID)
			m.log.Info("Adding new VM to warm pool", slog.Any("ip", vm.ip), slog.String("vmid", vm.vmmID))

			// If the pool is full, this line will block until a slot is available.
			m.warmVms <- vm

			// This gets executed when another goroutine pulls a vm out of the warmVms channel and unblocks
			m.allVms[vm.vmmID] = vm

			m.t.vmCounter.Add(m.ctx, 1)
		}
	}
}

func (m *MachineManager) awaitHandshake(vmid string) {
	timeoutAt := time.Now().UTC().Add(m.handshakeTimeout)

	handshakeOk := false
	for !handshakeOk && !m.stopping() {
		if time.Now().UTC().After(timeoutAt) {
			m.log.Error("Did not receive NATS handshake from agent within timeout.", slog.String("vmid", vmid))
			return
			// _ = m.Stop()
			// FIXME!!! os.Exit(1) // FIXME
		}

		_, handshakeOk = m.handshakes[vmid]
		time.Sleep(time.Millisecond * agentapi.DefaultRunloopSleepTimeoutMillis)
	}
}

// TODO : look into also pre-removing /var/lib/cni/networks/fcnet/ during startup sequence
// to ensure we get the full IP range

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

func (m *MachineManager) stopping() bool {
	return (atomic.LoadUint32(&m.closing) > 0)
}
