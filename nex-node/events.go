package nexnode

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	controlapi "github.com/ConnectEverything/nex/control-api"
	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

func (m *MachineManager) PublishCloudEvent(namespace string, event cloudevents.Event) {

	raw, _ := event.MarshalJSON()

	// $NEX.events.{namespace}.{event_type}
	err := m.nc.Publish(fmt.Sprintf("%s.%s.%s", EventSubjectPrefix, namespace, event.Type()), raw)
	if err != nil {
		m.log.WithError(err).Error("Failed to publish cloud event")
	}
	m.nc.Flush()
}

func (m *MachineManager) PublishMachineStopped(vm *runningFirecracker) {
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
		cloudevent.SetSource(m.PublicKey())
		cloudevent.SetID(uuid.NewString())
		cloudevent.SetTime(time.Now().UTC())
		cloudevent.SetType(agentapi.WorkloadStoppedEventType)
		cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
		_ = cloudevent.SetData(workloadStopped)

		m.PublishCloudEvent(vm.namespace, cloudevent)

		emitLog := emittedLog{
			Text:      "Workload stopped",
			Level:     logrus.DebugLevel,
			MachineId: vm.vmmID,
		}
		logBytes, _ := json.Marshal(emitLog)

		subject := fmt.Sprintf("%s.%s.%s.%s.%s", LogSubjectPrefix, vm.namespace, m.PublicKey(), workloadName, vm.vmmID)
		err := m.nc.Publish(subject, logBytes)
		if err != nil {
			m.log.WithError(err).Error("Failed to publish machine stopped event")
		}
		m.nc.Flush()
	}
}

func (m *MachineManager) PublishNodeStarted() {
	nodeStart := controlapi.NodeStartedEvent{
		Version: VERSION,
		Id:      m.PublicKey(),
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(m.PublicKey())
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType(controlapi.NodeStartedEventType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(nodeStart)

	m.PublishCloudEvent("system", cloudevent)
}

func (m *MachineManager) PublishNodeStopped() {
	cloudevent := cloudevents.NewEvent()

	m.log.Info("Publishing node stopped")
	evt := controlapi.NodeStoppedEvent{
		Id:       m.PublicKey(),
		Graceful: true,
	}

	cloudevent.SetSource(m.PublicKey())
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType(controlapi.NodeStoppedEventType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(evt)
	m.PublishCloudEvent("system", cloudevent)
}

type emittedLog struct {
	Text      string       `json:"text"`
	Level     logrus.Level `json:"level"`
	MachineId string       `json:"machine_id"`
}
