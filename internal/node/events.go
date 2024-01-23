package nexnode

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/google/uuid"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	controlapi "github.com/synadia-io/nex/internal/control-api"
)

// FIXME-- move this to types repo-- audit other places where it is redeclared (nex-cli)
type emittedLog struct {
	Text      string     `json:"text"`
	Level     slog.Level `json:"level"`
	MachineId string     `json:"machine_id"`
}

// PublishCloudEvent writes the given $NEX event to an arbitrary namespace
func (m *MachineManager) PublishCloudEvent(namespace string, event cloudevents.Event) error {
	raw, _ := event.MarshalJSON()

	// $NEX.events.{namespace}.{event_type}
	subject := fmt.Sprintf("%s.%s.%s", EventSubjectPrefix, namespace, event.Type())
	err := m.nc.Publish(subject, raw)
	if err != nil {
		m.log.Error("Failed to publish cloud event", slog.Any("err", err))
	}

	return m.nc.Flush()
}

// PublishMachineStopped writes a workload stopped event for the provided firecracker VM
func (m *MachineManager) PublishMachineStopped(vm *runningFirecracker) error {
	workloadName := strings.TrimSpace(vm.workloadSpecification.DecodedClaims.Subject)
	if len(workloadName) > 0 {
		workloadStopped := struct {
			Name   string `json:"name"`
			Reason string `json:"reason,omitempty"`
			VmId   string `json:"vmid"`
		}{
			Name:   workloadName,
			Reason: "Workload shutdown requested",
			VmId:   vm.vmmID,
		}

		cloudevent := cloudevents.NewEvent()
		cloudevent.SetSource(m.publicKey)
		cloudevent.SetID(uuid.NewString())
		cloudevent.SetTime(time.Now().UTC())
		cloudevent.SetType(agentapi.WorkloadStoppedEventType)
		cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
		_ = cloudevent.SetData(workloadStopped)

		err := m.PublishCloudEvent(vm.namespace, cloudevent)
		if err != nil {
			return err
		}

		emitLog := emittedLog{
			Text:      "Workload stopped",
			Level:     slog.LevelDebug,
			MachineId: vm.vmmID,
		}
		logBytes, _ := json.Marshal(emitLog)

		subject := fmt.Sprintf("%s.%s.%s.%s.%s", LogSubjectPrefix, vm.namespace, m.publicKey, workloadName, vm.vmmID)
		err = m.nc.Publish(subject, logBytes)
		if err != nil {
			m.log.Error("Failed to publish machine stopped event", slog.Any("err", err))
		}

		return m.nc.Flush()
	}

	return nil
}

// PublishNodeStarted emits a node_started event
func (m *MachineManager) PublishNodeStarted() error {
	nodeStart := controlapi.NodeStartedEvent{
		Version: VERSION,
		Id:      m.publicKey,
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(m.publicKey)
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType(controlapi.NodeStartedEventType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(nodeStart)

	return m.PublishCloudEvent("system", cloudevent)
}

// PublishNodeStopped emits a node_stopped event
func (m *MachineManager) PublishNodeStopped() error {
	evt := controlapi.NodeStoppedEvent{
		Id:       m.publicKey,
		Graceful: true,
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(m.publicKey)
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType(controlapi.NodeStoppedEventType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(evt)

	m.log.Info("Publishing node stopped")
	return m.PublishCloudEvent("system", cloudevent)
}
