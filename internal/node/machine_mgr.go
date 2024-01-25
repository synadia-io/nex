package nexnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	controlapi "github.com/synadia-io/nex/internal/control-api"
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

	return &MachineManager{
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
	}, nil
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

	go m.fillPool()
	_ = m.PublishNodeStarted()

	return nil
}

func (m *MachineManager) DeployWorkload(vm *runningFirecracker, workloadName, namespace string, request controlapi.RunRequest) error {
	// TODO: make the bytes and hash/digest available to the agent
	req := agentapi.DeployRequest{
		WorkloadName:    &workloadName,
		Hash:            nil, // FIXME
		TotalBytes:      nil, // FIXME
		TriggerSubjects: request.TriggerSubjects,
		WorkloadType:    request.WorkloadType, // FIXME-- audit all types for string -> *string, and validate...
		Environment:     request.WorkloadEnvironment,
	}
	bytes, _ := json.Marshal(req)

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
				intmsg.Header.Add("x-nex-trigger-subject", msg.Subject)

				resp, err := m.ncInternal.RequestMsg(intmsg, time.Millisecond*10000) // FIXME-- make timeout configurable
				if err != nil {
					m.log.Error("Failed to request agent execution via internal trigger subject",
						slog.Any("err", err),
						slog.String("trigger_subject", tsub),
						slog.String("workload_type", *request.WorkloadType),
						slog.String("vmid", vm.vmmID),
					)
				} else if resp != nil {
					m.log.Debug("Received response from execution via trigger subject",
						slog.String("vmid", vm.vmmID),
						slog.String("trigger_subject", tsub),
						slog.String("workload_type", *request.WorkloadType),
						slog.Int("payload_size", len(resp.Data)),
					)
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
	vm.namespace = namespace
	vm.workloadSpecification = request

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
				// if we can't create a vm, there's no point in this app staying up
				panic(err)
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
