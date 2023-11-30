package nexnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	controlapi "github.com/ConnectEverything/nex/control-api"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/sirupsen/logrus"
)

const (
	EventSubjectPrefix = "$NEX.events"
	LogSubjectPrefix   = "$NEX.logs"
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
	log         *logrus.Logger
	allVms      map[string]*runningFirecracker
	warmVms     chan *runningFirecracker
}

func NewMachineManager(ctx context.Context, cancel context.CancelFunc, nc *nats.Conn, config *NodeConfiguration, log *logrus.Logger) *MachineManager {
	// Create a new User KeyPair
	server, _ := nkeys.CreateServer()
	return &MachineManager{
		rootContext: ctx,
		rootCancel:  cancel,
		config:      config,
		nc:          nc,
		log:         log,
		kp:          server,
		allVms:      make(map[string]*runningFirecracker),
		warmVms:     make(chan *runningFirecracker, config.MachinePoolSize-1),
	}
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
	m.log.WithField("client_url", natsServer.ClientURL()).Info("Internal NATS server started")

	_, err = m.ncInternal.Subscribe("agentint.*.logs", handleAgentLog(m))
	if err != nil {
		return err
	}
	_, err = m.ncInternal.Subscribe("agentint.*.events.*", handleAgentEvent(m))
	if err != nil {
		return err
	}
	_, err = m.ncInternal.Subscribe("agentint.advertise", handleAdvertise(m))
	if err != nil {
		return err
	}
	go m.fillPool()
	m.PublishNodeStarted()

	return nil
}

func (m *MachineManager) DispatchWork(vm *runningFirecracker, workloadName string, namespace string, request controlapi.RunRequest) error {

	// TODO: make the bytes and hash/digest available to the agent

	req := agentapi.WorkRequest{
		WorkloadName: workloadName,
		Hash:         "",
		TotalBytes:   0, // TODO: make real
		WorkloadType: request.WorkloadType,
		Environment:  request.WorkloadEnvironment,
	}
	bytes, _ := json.Marshal(req)

	subject := fmt.Sprintf("agentint.%s.workdispatch", vm.vmmID)
	resp, err := m.ncInternal.Request(subject, bytes, 1*time.Second)
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return errors.New("timed out waiting for acknowledgement of work dispatch")
		} else {
			return fmt.Errorf("failed to submit request for work: %s", err)
		}
	}
	var workResponse agentapi.WorkResponse
	err = json.Unmarshal(resp.Data, &workResponse)
	if err != nil {
		return err
	}
	if !workResponse.Accepted {
		return fmt.Errorf("workload rejected by agent: %s", workResponse.Message)
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
		m.PublishMachineStopped(vm)
		vm.shutDown()
	}
	m.PublishNodeStopped()
	// Now empty the leftovers in the pool
	for vm := range m.warmVms {
		vm.shutDown()
	}
	time.Sleep(100 * time.Millisecond)

	m.nc.Drain()

	return nil
}

// Stops a single machine. Will return an error if called with a non-existent workload/vm ID
func (m *MachineManager) StopMachine(vmId string) error {
	vm, exists := m.allVms[vmId]
	if !exists {
		return errors.New("no such workload")
	}
	m.PublishMachineStopped(vm)
	vm.shutDown()
	delete(m.allVms, vmId)
	return nil
}

// Retrieves the machine manager's public key, which comes from a key pair of type server (Nxxx)
func (m *MachineManager) PublicKey() string {
	pk, err := m.kp.PublicKey()
	if err != nil {
		return "???"
	} else {
		return pk
	}
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
			vm, err := createAndStartVM(m.rootContext, m.config)
			if err != nil {
				m.log.WithError(err).Error("Failed to create VMM for warming pool. Aborting.")
				// if we can't create a vm, there's no point in this app staying up
				panic(err)
			}

			m.log.WithField("ip", vm.ip).WithField("vmid", vm.vmmID).Info("Adding new VM to warm pool")

			// Don't wait forever, if the VM is not available after 5s, move on
			//ctx, cancel := context.WithTimeout(m.rootContext, 5*time.Second)
			//defer cancel()

			// err = vm.agentClient.WaitForAgentToBoot(ctx)
			// if err != nil {
			// 	m.log.WithError(err).Info("VM did not respond with healthy in timeout. Aborting.")
			// 	vm.vmmCancel()
			// 	break
			// }

			// logs, events, err := vm.Subscribe()
			// if err != nil {
			// 	m.log.WithError(err).Error("Failed to get channels from client for gRPC subscriptions")
			// 	return
			// }

			// go dispatchLogs(vm, m.kp, m.nc, logs, m.log)
			// go dispatchEvents(m, vm, m.kp, m.nc, events, m.log)

			// // Add the new microVM to the pool.
			// // If the pool is full, this line will block until a slot is available.
			m.warmVms <- vm
			// // This gets executed when another goroutine pulls a vm out of the warmVms channel and unblocks
			m.allVms[vm.vmmID] = vm
		}
	}
}

/*
func dispatchLogs(vm *runningFirecracker, kp nkeys.KeyPair, nc *nats.Conn, logs <-chan *agentapi.LogEntry, log *logrus.Logger) {
	pk, _ := kp.PublicKey()
	for {
		entry := <-logs
		workloadName := vm.workloadSpecification.DecodedClaims.Subject
		lvl := getLogrusLevel(entry)

		if len(strings.TrimSpace(workloadName)) != 0 {
			emitLog := emittedLog{
				Text:      entry.Text,
				Level:     lvl,
				MachineId: vm.vmmID,
			}
			logBytes, _ := json.Marshal(emitLog)
			// $NEX.LOGS.{namespace}.{host}.{workload}.{vm}
			subject := fmt.Sprintf("%s.%s.%s.%s.%s", logSubjectPrefix, vm.namespace, pk, workloadName, vm.vmmID)
			err := nc.Publish(subject, logBytes)
			if err != nil {
				log.WithField("vmid", vm.vmmID).WithField("ip", vm.ip).WithField("subject", subject).Warn("Failed to publish log on logs subject")
			}
		}

		log.WithField("namespace", vm.namespace).WithField("vmid", vm.vmmID).WithField("ip", vm.ip).Log(lvl, entry.Text)

	}
}

func dispatchEvents(m *MachineManager, vm *runningFirecracker, kp nkeys.KeyPair, nc *nats.Conn, events <-chan *agentapi.AgentEvent, log *logrus.Logger) {
	pk, _ := kp.PublicKey()
	for {
		event := <-events
		eventType := getEventType(event)
		m.PublishCloudEvent(eventType, vm.namespace, agentEventToCloudEvent(vm, pk, event, eventType))
		log.WithField("vmid", vm.vmmID).
			WithField("event_type", eventType).
			WithField("ip", vm.ip).
			WithField("namespace", vm.namespace).
			Debug("Received event from agent")
		if eventType == "agent_stopped" || eventType == "workload_stopped" {
			delete(m.allVms, vm.vmmID)
			vm.shutDown()
		}
	}
}

func getEventType(event *agentapi.AgentEvent) string {
	var eventType string
	switch event.Data.(type) {
	case *agentapi.AgentEvent_AgentStarted:
		eventType = "agent_started"
	case *agentapi.AgentEvent_AgentStopped:
		eventType = "agent_stopped"
	case *agentapi.AgentEvent_WorkloadStarted:
		eventType = "workload_started"
	case *agentapi.AgentEvent_WorkloadStopped:
		eventType = "workload_stopped"
	default:
		eventType = "unknown"
	}
	return eventType
}

func getLogrusLevel(entry *agentapi.LogEntry) logrus.Level {
	switch entry.Level {
	case agentapi.LogLevel_LEVEL_INFO:
		return logrus.InfoLevel
	case agentapi.LogLevel_LEVEL_DEBUG:
		return logrus.DebugLevel
	case agentapi.LogLevel_LEVEL_PANIC:
		return logrus.PanicLevel
	case agentapi.LogLevel_LEVEL_FATAL:
		return logrus.FatalLevel
	case agentapi.LogLevel_LEVEL_WARN:
		return logrus.WarnLevel
	case agentapi.LogLevel_LEVEL_TRACE:
		return logrus.TraceLevel
	default:
		return logrus.DebugLevel
	}
}

func agentEventToCloudEvent(vm *runningFirecracker, pk string, event *agentapi.AgentEvent, eventType string) cloudevents.Event {
	cloudevent := cloudevents.NewEvent()

	cloudevent.SetSource(fmt.Sprintf("%s-%s", pk, vm.vmmID))
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType(eventType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)

	// ACL so we don't bleed inner details of control mechanisms onto public event streams

	var data interface{}
	switch typ := event.Data.(type) {
	case *agentapi.AgentEvent_AgentStarted:
		data = controlapi.AgentStartedEvent{
			AgentVersion: typ.AgentStarted.AgentVersion,
		}
	case *agentapi.AgentEvent_AgentStopped:
		data = controlapi.AgentStoppedEvent{
			Message: typ.AgentStopped.Message,
			Code:    int(typ.AgentStopped.Code),
		}
	case *agentapi.AgentEvent_WorkloadStarted:
		data = controlapi.WorkloadStartedEvent{
			Name:       typ.WorkloadStarted.Name,
			TotalBytes: int(typ.WorkloadStarted.TotalBytes),
		}
	case *agentapi.AgentEvent_WorkloadStopped:
		data = controlapi.WorkloadStoppedEvent{
			Name:    typ.WorkloadStopped.Name,
			Code:    int(typ.WorkloadStopped.Code),
			Message: typ.WorkloadStopped.Message,
		}
	default:
		data = struct{}{}
	}

	cloudevent.SetData(data)

	return cloudevent
}
*/

func (m *MachineManager) startInternalNats() (*server.Server, *nats.Conn, error) {

	natsServer, err := server.NewServer(&server.Options{
		Host:      "0.0.0.0",
		Port:      m.config.InternalNodePort,
		JetStream: true,
		NoLog:     true,
		StoreDir:  path.Join(os.TempDir(), "pnats"),
	})
	if err != nil {
		return nil, nil, err
	}
	natsServer.Start()
	time.Sleep(50 * time.Millisecond) // TODO: unsure if we need to give the server time to start

	nc, err := nats.Connect(fmt.Sprintf("nats://0.0.0.0:%d", m.config.InternalNodePort))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to internal nats: %s", err)
	}

	jsCtx, err := nc.JetStream()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to establish jetstream connection to internal nats: %s", err)
	}

	_, err = jsCtx.CreateObjectStore(&nats.ObjectStoreConfig{
		Bucket:      "NEXCACHE",
		Description: "Object store cache for nex-node workloads",
		Storage:     nats.MemoryStorage,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create internal object store: %s", err)
	}

	return natsServer, nc, nil
}
