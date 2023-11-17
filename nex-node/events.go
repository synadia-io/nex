package nexnode

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

func (m *MachineManager) PublishCloudEvent(eventType string, namespace string, event cloudevents.Event) {
	event.SetType(eventType) // force this to always be consistent

	raw, err := event.MarshalJSON()
	if err != nil {
		panic(err)
	}

	// TODO: in the future, consider using a queue so we can buffer events when
	// disconnected
	m.nc.Publish(fmt.Sprintf("%s.%s.%s", eventSubjectPrefix, namespace, eventType), raw)
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
			Reason: "Node shutdown",
			VmId:   vm.vmmID,
		}

		cloudevent := cloudevents.NewEvent()
		cloudevent.SetSource(m.PublicKey())
		cloudevent.SetID(uuid.NewString())
		cloudevent.SetTime(time.Now().UTC())
		cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
		cloudevent.SetData(workloadStopped)

		m.PublishCloudEvent("workload_stopped", vm.namespace, cloudevent)

		emitLog := emittedLog{
			Text:      "Workload stopped",
			Level:     logrus.DebugLevel,
			MachineId: vm.vmmID,
		}
		logBytes, _ := json.Marshal(emitLog)

		subject := fmt.Sprintf("%s.%s.%s.%s.%s", logSubjectPrefix, vm.namespace, m.PublicKey(), workloadName, vm.vmmID)
		m.nc.Publish(subject, logBytes)
		m.nc.Flush()
	}
}

func (m *MachineManager) PublishNodeStarted() {
	nodeStart := struct {
		Version string `json:"version"`
		Id      string `json:"id"`
	}{
		Version: VERSION,
		Id:      m.PublicKey(),
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(m.PublicKey())
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType("node_stopped")
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	cloudevent.SetData(nodeStart)

	m.PublishCloudEvent("node_started", "system", cloudevent)
}

func (m *MachineManager) PublishNodeStopped() {
	cloudevent := cloudevents.NewEvent()

	// TODO: move this to shared struct if we add fields
	evt := struct {
		Id       string `json:"id"`
		Graceful bool   `json:"graceful"`
	}{
		Id:       m.PublicKey(),
		Graceful: true,
	}

	cloudevent.SetSource(m.PublicKey())
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType("node_stopped")
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	cloudevent.SetData(evt)
	m.PublishCloudEvent("node_stopped", "system", cloudevent)
}

type emittedLog struct {
	Text      string       `json:"text"`
	Level     logrus.Level `json:"level"`
	MachineId string       `json:"machine_id"`
}
