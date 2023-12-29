package controlapi

import (
	"encoding/json"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

func TestEventMonitor(t *testing.T) {
	nc, _ := nats.Connect(nats.DefaultURL)
	log := logrus.New()
	apiClient := NewApiClient(nc, time.Second, log)
	ch, err := apiClient.MonitorEvents("*", "*", 0)
	if err != nil {
		t.Fatalf("Failed to create log monitor: %s", err)
	}

	type testStruct struct {
		Bob   string `json:"bob"`
		Alice string `json:"alice"`
	}

	evt := cloudevents.NewEvent()
	evt.SetType("workload_started")
	evt.SetID("1")
	evt.SetSource("testing")
	_ = evt.SetData(testStruct{
		Bob:   "1",
		Alice: "2",
	})

	bytes, _ := json.Marshal(evt)
	_ = nc.Publish("$NEX.events.default.workload_started", bytes)

	actualEvent := <-ch
	if actualEvent.EventType != "workload_started" {
		t.Fatal("Event wrapper didn't maintain event type")
	}
	if actualEvent.Namespace != "default" {
		t.Fatal("Event wrapper didn't maintain namespace")
	}
	var ts testStruct
	err = actualEvent.DataAs(&ts)
	if err != nil {
		t.Fatalf("Event wrapper lost fidelity of event data: %s", err)
	}
	if ts.Alice != "2" || ts.Bob != "1" {
		t.Fatalf("Lost data in event round trip!: %+v", ts)
	}
}

func TestLogMonitor(t *testing.T) {
	nc, _ := nats.Connect(nats.DefaultURL)
	log := logrus.New()
	apiClient := NewApiClient(nc, time.Second, log)
	ch, err := apiClient.MonitorLogs("*", "*", "*", "*", 0)
	if err != nil {
		t.Fatalf("Failed to create log monitor: %s", err)
	}
	rawLog := rawLog{Text: "hey from test", Level: logrus.DebugLevel, MachineId: "vm1234"}
	bytes, _ := json.Marshal(rawLog)

	_ = nc.Publish("$NEX.logs.default.Nxxxx.echoservice.vm1234", bytes)
	actualEntry := <-ch
	if actualEntry.Namespace != "default" {
		t.Fatalf("namespace in log should be default, found %s", actualEntry.Namespace)
	}
	if actualEntry.NodeId != "Nxxxx" {
		t.Fatalf("node ID failed to propogate, should be Nxxx found %s", actualEntry.NodeId)
	}
	if actualEntry.Workload != "echoservice" {
		t.Fatalf("workload failed to propogate, should be echoservice, found %s", actualEntry.Workload)
	}
	if actualEntry.MachineId != "vm1234" {
		t.Fatalf("did not get the right machine ID. expected vm1234, found %s", actualEntry.MachineId)
	}
	if actualEntry.rawLog != rawLog {
		t.Fatalf("Failed to wrap the raw on the wire log: %+v", actualEntry)
	}
}
