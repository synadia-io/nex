package nexnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	controlapi "github.com/synadia-io/nex/internal/control-api"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	EventSubjectPrefix      = "$NEX.events"
	LogSubjectPrefix        = "$NEX.logs"
	WorkloadCacheBucketName = "NEXCACHE"

	defaultHandshakeTimeoutMillis = 5000
	defaultNatsStoreDir           = "pnats"
)

// The machine manager is responsible for the pool of warm firecracker VMs. This includes starting new
// VMs, stopping VMs, and pulling VMs from the pool on demand
type MachineManager struct {
	api         *ApiListener
	rootContext context.Context
	rootCancel  context.CancelFunc
	config      *NodeConfiguration
	kp          nkeys.KeyPair
	nc          *nats.Conn
	ncInternal  *nats.Conn
	log         *slog.Logger
	allVms      map[string]*runningFirecracker
	warmVms     chan *runningFirecracker

	handshakes       map[string]string
	handshakeTimeout time.Duration // TODO: make configurable...

	natsStoreDir string
	publicKey    string
}

func NewMachineManager(ctx context.Context, cancel context.CancelFunc, nc *nats.Conn, config *NodeConfiguration, log *slog.Logger) (*MachineManager, error) {
	// Validate the node config
	if !config.Validate() {
		return nil, fmt.Errorf("failed to create new machine manager; invalid node config; %v", config.Errors)
	}

	// Create a new keypair
	server, err := nkeys.CreateServer()
	if err != nil {
		return nil, fmt.Errorf("failed to create new machine manager; failed to generate keypair; %s", err)
	}

	pubkey, err := server.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to create new machine manager; failed to encode public key; %s", err)
	}

	m := &MachineManager{
		rootContext:      ctx,
		rootCancel:       cancel,
		config:           config,
		nc:               nc,
		log:              log,
		kp:               server,
		publicKey:        pubkey,
		handshakes:       make(map[string]string),
		handshakeTimeout: time.Duration(defaultHandshakeTimeoutMillis * time.Millisecond),
		natsStoreDir:     defaultNatsStoreDir,
		allVms:           make(map[string]*runningFirecracker),
		warmVms:          make(chan *runningFirecracker, config.MachinePoolSize-1),
	}

	m.api = NewApiListener(log, m, config)

	return m, nil
}

// Starts the machine manager. Publishes a node started event and starts the goroutine responsible for
// keeping the firecracker VM pool full
func (m *MachineManager) Start() error {
	m.log.Info("Virtual machine manager starting")

	natsServer, ncInternal, err := m.startInternalNats()
	if err != nil {
		return err
	}

	m.ncInternal = ncInternal
	m.log.Info("Internal NATs server started", slog.String("client_url", natsServer.ClientURL()))

	_, err = m.ncInternal.Subscribe("agentint.*.logs", handleAgentLog(m))
	if err != nil {
		return err
	}

	_, err = m.ncInternal.Subscribe("agentint.*.events.*", handleAgentEvent(m))
	if err != nil {
		return err
	}

	_, err = m.ncInternal.Subscribe("agentint.handshake", handleHandshake(m))
	if err != nil {
		return err
	}

	err = m.api.Start()
	if err != nil {
		m.log.Error("Failed to start API listener", slog.Any("err", err))
		return err
	}

	go m.fillPool()
	_ = m.PublishNodeStarted()

	m.setupSignalHandlers()

	return nil
}

func (m *MachineManager) DeployWorkload(vm *runningFirecracker, runRequest controlapi.RunRequest, deployRequest agentapi.DeployRequest) error {
	bytes, err := json.Marshal(deployRequest)
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
	} else if runRequest.SupportsTriggerSubjects() {
		for _, tsub := range runRequest.TriggerSubjects {
			_, err := m.nc.Subscribe(tsub, func(msg *nats.Msg) {
				intmsg := nats.NewMsg(fmt.Sprintf("agentint.%s.trigger", vm.vmmID))
				intmsg.Data = msg.Data
				intmsg.Header.Add("x-nex-trigger-subject", msg.Subject)

				resp, err := m.ncInternal.RequestMsg(intmsg, time.Millisecond*10000) // FIXME-- make timeout configurable
				if err != nil {
					m.log.Error("Failed to request agent execution via internal trigger subject",
						slog.Any("err", err),
						slog.String("trigger_subject", tsub),
						slog.String("workload_type", *runRequest.WorkloadType),
						slog.String("vmid", vm.vmmID),
					)
				} else if resp != nil {
					m.log.Debug("Received response from execution via trigger subject",
						slog.String("vmid", vm.vmmID),
						slog.String("trigger_subject", tsub),
						slog.String("workload_type", *runRequest.WorkloadType),
						slog.Int("payload_size", len(resp.Data)),
					)
					err = msg.Respond(resp.Data)
					if err != nil {
						m.log.Error("Failed to respond to trigger subject subscription request for deployed workload",
							slog.String("vmid", vm.vmmID),
							slog.String("trigger_subject", tsub),
							slog.String("workload_type", *runRequest.WorkloadType),
							slog.Any("err", err),
						)
					}
				}
			})
			if err != nil {
				m.log.Error("Failed to create trigger subject subscription for deployed workload",
					slog.String("vmid", vm.vmmID),
					slog.String("trigger_subject", tsub),
					slog.String("workload_type", *runRequest.WorkloadType),
					slog.Any("err", err),
				)
				// TODO-- rollback the otherwise accepted deployment and return the error below...
				// return err
			}

			m.log.Info("Created trigger subject subscription for deployed workload",
				slog.String("vmid", vm.vmmID),
				slog.String("trigger_subject", tsub),
				slog.String("workload_type", *runRequest.WorkloadType),
			)
		}
	}

	vm.workloadStarted = time.Now().UTC()
	vm.namespace = *deployRequest.Namespace
	vm.workloadSpecification = runRequest
	vm.deployedWorkload = deployRequest

	workloadCounter.Add(m.rootContext, 1)
	workloadCounter.Add(m.rootContext, 1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	deployedByteCounter.Add(m.rootContext, deployRequest.TotalBytes)
	deployedByteCounter.Add(m.rootContext, deployRequest.TotalBytes, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	allocatedVCPUCounter.Add(m.rootContext, *vm.machine.Cfg.MachineCfg.VcpuCount)
	allocatedVCPUCounter.Add(m.rootContext, *vm.machine.Cfg.MachineCfg.VcpuCount, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	allocatedMemoryCounter.Add(m.rootContext, *vm.machine.Cfg.MachineCfg.MemSizeMib)
	allocatedMemoryCounter.Add(m.rootContext, *vm.machine.Cfg.MachineCfg.MemSizeMib, metric.WithAttributes(attribute.String("namespace", vm.namespace)))

	return nil
}

// Stops the machine manager, which will in turn stop all firecracker VMs and attempt to clean
// up any applicable resources. Note that all "stopped" events emitted during a stop are best-effort
// and not guaranteed.
func (m *MachineManager) Stop() error {
	m.log.Info("Virtual machine manager stopping")

	m.rootCancel() // stops the pool from refilling
	for _, vm := range m.allVms {
		_ = m.PublishMachineStopped(vm)
		vm.shutDown(m.log)
	}

	_ = m.PublishNodeStopped()

	// Now empty the leftovers in the pool
	for vm := range m.warmVms {
		vm.shutDown(m.log)
		// TODO: confirm this needs to be here
		workloadCounter.Add(m.rootContext, -1)
		workloadCounter.Add(m.rootContext, -1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
		deployedByteCounter.Add(m.rootContext, vm.deployedWorkload.TotalBytes*-1)
		deployedByteCounter.Add(m.rootContext, vm.deployedWorkload.TotalBytes*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
		allocatedVCPUCounter.Add(m.rootContext, *vm.machine.Cfg.MachineCfg.VcpuCount*-1)
		allocatedVCPUCounter.Add(m.rootContext, *vm.machine.Cfg.MachineCfg.VcpuCount*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
		allocatedMemoryCounter.Add(m.rootContext, *vm.machine.Cfg.MachineCfg.MemSizeMib*-1)
		allocatedMemoryCounter.Add(m.rootContext, *vm.machine.Cfg.MachineCfg.MemSizeMib*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	}
	time.Sleep(100 * time.Millisecond)

	_ = m.nc.Drain()

	return nil
}

// Stops a single machine. Will return an error if called with a non-existent workload/vm ID
func (m *MachineManager) StopMachine(vmId string) error {
	vm, exists := m.allVms[vmId]
	if !exists {
		return errors.New("no such workload")
	}

	subject := fmt.Sprintf("agentint.%s.undeploy", vm.vmmID)
	// we do a request here so we don't tell firecracker to shut down until
	// we get a reply
	_, err := m.ncInternal.Request(subject, []byte{}, 500*time.Millisecond)
	if err != nil {
		return err
	}

	_ = m.PublishMachineStopped(vm)
	vm.shutDown(m.log)
	delete(m.allVms, vmId)

	workloadCounter.Add(m.rootContext, -1)
	workloadCounter.Add(m.rootContext, -1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	deployedByteCounter.Add(m.rootContext, vm.deployedWorkload.TotalBytes*-1)
	deployedByteCounter.Add(m.rootContext, vm.deployedWorkload.TotalBytes*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	allocatedVCPUCounter.Add(m.rootContext, *vm.machine.Cfg.MachineCfg.VcpuCount*-1)
	allocatedVCPUCounter.Add(m.rootContext, *vm.machine.Cfg.MachineCfg.VcpuCount*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))
	allocatedMemoryCounter.Add(m.rootContext, *vm.machine.Cfg.MachineCfg.MemSizeMib*-1)
	allocatedMemoryCounter.Add(m.rootContext, *vm.machine.Cfg.MachineCfg.MemSizeMib*-1, metric.WithAttributes(attribute.String("namespace", vm.namespace)))

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
	for {
		select {
		case <-m.rootContext.Done():
			return
		default:
			vm, err := createAndStartVM(m.rootContext, m.config, m.log)
			if err != nil {
				m.log.Error("Failed to create VMM for warming pool. Aborting.", slog.Any("err", err))
				return
			}
			go m.awaitHandshake(vm.vmmID)
			m.log.Info("Adding new VM to warm pool", slog.Any("ip", vm.ip), slog.String("vmid", vm.vmmID))

			// If the pool is full, this line will block until a slot is available.
			m.warmVms <- vm

			// This gets executed when another goroutine pulls a vm out of the warmVms channel and unblocks
			m.allVms[vm.vmmID] = vm
		}
	}
}

func (m *MachineManager) awaitHandshake(vmid string) {
	timeoutAt := time.Now().UTC().Add(m.handshakeTimeout)

	handshakeOk := false
	for !handshakeOk {
		if time.Now().UTC().After(timeoutAt) {
			m.log.Error("Did not receive NATS handshake from agent within timeout. Exiting unstable node", slog.String("vmid", vmid))
			_ = m.Stop()
			os.Exit(1) // FIXME
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

func (m *MachineManager) setupSignalHandlers() {
	go func() {
		// both firecracker and the embedded NATS server register signal handlers... wipe those so ours are the ones being used
		signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGHUP)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)

		stop := func() {
			err := m.Stop()
			if err != nil {
				m.log.Warn("Machine manager failed to stop", slog.Any("err", err))
			}

			m.cleanSockets()
			os.Exit(0) // FIXME
		}

		for {
			switch s := <-c; {
			case s == syscall.SIGTERM || s == os.Interrupt:
				m.log.Info("Caught signal, requesting clean shutdown", slog.String("signal", s.String()))
				stop()
			case s == syscall.SIGQUIT:
				m.log.Info("Caught quit signal, still trying graceful shutdown", slog.String("signal", s.String()))
				stop()
			}
		}
	}()
}

func (m *MachineManager) startInternalNats() (*server.Server, *nats.Conn, error) {
	natsServer, err := server.NewServer(&server.Options{
		Host:      "0.0.0.0",
		Port:      -1,
		JetStream: true,
		NoLog:     true,
		StoreDir:  path.Join(os.TempDir(), m.natsStoreDir),
	})
	if err != nil {
		return nil, nil, err
	}
	natsServer.Start()
	time.Sleep(50 * time.Millisecond) // TODO: unsure if we need to give the server time to start

	clientUrl, err := url.Parse(natsServer.ClientURL())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse internal NATS client URL: %s", err)
	}
	p, err := strconv.Atoi(clientUrl.Port())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse internal NATS client URL: %s", err)
	}
	m.config.InternalNodePort = &p
	nc, err := nats.Connect(natsServer.ClientURL())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to internal nats: %s", err)
	}

	jsCtx, err := nc.JetStream()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to establish jetstream connection to internal nats: %s", err)
	}

	_, err = jsCtx.CreateObjectStore(&nats.ObjectStoreConfig{
		Bucket:      WorkloadCacheBucketName,
		Description: "Object store cache for nex-node workloads",
		Storage:     nats.MemoryStorage,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create internal object store: %s", err)
	}

	return natsServer, nc, nil
}
