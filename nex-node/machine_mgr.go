package nexnode

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/sirupsen/logrus"
)

const (
	eventSubjectPrefix = "$NEX.events"
	logSubjectPrefix   = "$NEX.logs"
)

type MachineManagerConfiguration struct {
	MachinePoolSize int
	RootFsPath      string
	KernelPath      string
}

type MachineManager struct {
	rootContext context.Context
	rootCancel  context.CancelFunc
	config      *NodeConfiguration
	kp          nkeys.KeyPair
	nc          *nats.Conn
	log         *logrus.Logger
	allVms      map[string]runningFirecracker
	warmVms     chan runningFirecracker
}

type emittedLog struct {
	Text      string       `json:"text"`
	Level     logrus.Level `json:"level"`
	MachineId string       `json:"machine_id"`
}

func (m *MachineManager) Start() error {
	m.log.Info("Virtual machine manager starting")
	go m.fillPool()

	return nil
}

func (m *MachineManager) Stop() error {
	m.log.Info("Virtual machine manager stopping")
	// TODO: move this to shared struct if we add fields
	evt := struct {
		Id       string `json:"id"`
		Graceful bool   `json:"graceful"`
	}{
		Id:       m.PublicKey(),
		Graceful: true,
	}
	m.PublishCloudEvent("node_stopped", evt)

	m.rootCancel() // stops the pool from refilling
	for _, vm := range m.allVms {
		vm.shutDown()
	}
	// Now empty the leftovers in the pool
	for vm := range m.warmVms {
		vm.shutDown()
	}

	return nil
}

func (m *MachineManager) PublicKey() string {
	pk, err := m.kp.PublicKey()
	if err != nil {
		return "???"
	} else {
		return pk
	}
}

func (m *MachineManager) TakeFromPool() (*runningFirecracker, error) {
	running := <-m.warmVms
	m.allVms[running.vmmID] = running
	return &running, nil
}

func (m *MachineManager) PublishCloudEvent(eventType string, data interface{}) {
	pk, _ := m.kp.PublicKey()
	cloudevent := cloudevents.NewEvent()
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetType(eventType)
	cloudevent.SetSource(pk)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	cloudevent.SetData(data)
	raw, err := cloudevent.MarshalJSON()
	if err != nil {
		panic(err)
	}

	// TODO: in the future, consider using a queue so we can buffer events when
	// disconnected
	m.nc.Publish(fmt.Sprintf("%s.%s", eventSubjectPrefix, eventType), raw)
	m.nc.Flush()
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
				time.Sleep(time.Second)
				break
			}

			m.log.WithField("ip", vm.ip).WithField("vmid", vm.vmmID).Info("Adding new VM to warm pool")

			// Don't wait forever, if the VM is not available after 5s, move on
			ctx, cancel := context.WithTimeout(m.rootContext, 5*time.Second)
			defer cancel()

			err = vm.agentClient.WaitForAgentToBoot(ctx)
			if err != nil {
				m.log.WithError(err).Info("VM did not respond with healthy in timeout. Aborting.")
				vm.vmmCancel()
				break
			}

			logs, events, err := vm.Subscribe()
			if err != nil {
				m.log.WithError(err).Error("Failed to get channels from client for gRPC subscriptions")
				return
			}
			go dispatchLogs(vm, m.kp, m.nc, logs, m.log)
			go dispatchEvents(vm, m.kp, m.nc, events, m.log)

			// Add the new microVM to the pool.
			// If the pool is full, this line will block until a slot is available.
			m.warmVms <- *vm
			// This gets executed when another goroutine pulls a vm out of the warmVms channel and unblocks
			m.allVms[vm.vmmID] = *vm
		}
	}
}

func dispatchLogs(vm *runningFirecracker, kp nkeys.KeyPair, nc *nats.Conn, logs <-chan *agentapi.LogEntry, log *logrus.Logger) {
	pk, _ := kp.PublicKey()
	for {
		entry := <-logs
		lvl := getLogrusLevel(entry)
		emitLog := emittedLog{
			Text:      entry.Text,
			Level:     lvl,
			MachineId: vm.vmmID,
		}
		logBytes, _ := json.Marshal(emitLog)
		workloadName := vm.workloadSpecification.DecodedClaims.Subject
		// $NEX.LOGS.{host}.{workload}.{vm}
		nc.Publish(fmt.Sprintf("%s.%s.%s.%s", logSubjectPrefix, pk, workloadName, vm.vmmID), logBytes)
		log.WithField("vmid", vm.vmmID).WithField("ip", vm.ip).Log(lvl, entry.Text)

	}
}

func dispatchEvents(vm *runningFirecracker, kp nkeys.KeyPair, nc *nats.Conn, events <-chan *agentapi.AgentEvent, log *logrus.Logger) {
	pk, _ := kp.PublicKey()
	for {
		event := <-events
		eventType := getEventType(event)
		nc.Publish(fmt.Sprintf("%s.%s", eventSubjectPrefix, eventType), agentEventToCloudEvent(vm, pk, event, eventType))
		log.WithField("vmid", vm.vmmID).
			WithField("event_type", eventType).
			WithField("ip", vm.ip).
			Info("Received event from agent")
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

func agentEventToCloudEvent(vm *runningFirecracker, pk string, event *agentapi.AgentEvent, eventType string) []byte {
	cloudevent := cloudevents.NewEvent()

	cloudevent.SetSource(fmt.Sprintf("%s-%s", pk, vm.vmmID))
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetType(eventType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	cloudevent.SetData(event.Data)

	raw, _ := cloudevent.MarshalJSON()

	return raw
}
