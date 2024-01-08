package nexnode

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

// Called when the node server gets a log entry via internal NATS. Used to
// package and re-mit with additional metadata on $NEX.logs...
func handleAgentLog(mgr *MachineManager) func(m *nats.Msg) {
	return func(m *nats.Msg) {
		tokens := strings.Split(m.Subject, ".")
		vmId := tokens[1]

		vm, ok := mgr.allVms[vmId]
		if !ok {
			mgr.log.Warn("Received a log from a VM we don't know about. Rejecting")
			return
		}

		var logentry agentapi.LogEntry
		err := json.Unmarshal(m.Data, &logentry)
		if err != nil {
			mgr.log.WithError(err).Error("Failed to unmarshal log entry from agent")
			return
		}

		outLog := emittedLog{
			Text:      logentry.Text,
			Level:     logrus.Level(logentry.Level),
			MachineId: vmId,
		}

		bytes, err := json.Marshal(outLog)
		if err != nil {
			mgr.log.WithError(err).Error("Failed to marshal our own log entry!")
			return
		}

		_ = mgr.nc.Publish(logPublishSubject(vm.namespace, mgr.publicKey, vm.workloadSpecification.DecodedClaims.Subject, vmId), bytes)
	}
}

// Called when the node server gets an event from the nex agent inside firecracker. The data here is already a fully formed
// cloud event, so all we need to do is unmarshal it, get some metadata, and then republish on $NEX.events...
func handleAgentEvent(mgr *MachineManager) func(m *nats.Msg) {
	return func(m *nats.Msg) {
		// agentint.{vmid}.events.{type}
		tokens := strings.Split(m.Subject, ".")
		vmId := tokens[1]

		vm, ok := mgr.allVms[vmId]
		if !ok {
			mgr.log.Warn("Received an event from a VM we don't know about. Rejecting.")
			return
		}

		var evt cloudevents.Event
		err := json.Unmarshal(m.Data, &evt)
		if err != nil {
			mgr.log.WithError(err).Error("Failed to deserialize cloudevent from agent")
			return
		}
		mgr.log.WithField("vmid", vmId).WithField("type", evt.Type()).Info("Received agent event")

		err = mgr.PublishCloudEvent(vm.namespace, evt)
		if err != nil {
			mgr.log.WithError(err).Error("Failed to publish cloudevent")
			return
		}

		if evt.Type() == agentapi.WorkloadStoppedEventType {
			vm.shutDown()
		}
	}
}

// This handshake uses the request pattern to force a full round trip to ensure connectivity is working properly as
// fire-and-forget publishes from inside the firecracker VM could potentially be lost
func handleHandshake(mgr *MachineManager) func(m *nats.Msg) {
	return func(m *nats.Msg) {
		var shake agentapi.HandshakeRequest
		err := json.Unmarshal(m.Data, &shake)
		if err != nil {
			mgr.log.WithField("vmid", *shake.MachineId).WithField("message", *shake.Message).Error("Failed to handle agent handshake")
			return
		}

		now := time.Now().UTC()
		mgr.handshakes[*shake.MachineId] = now.Format(time.RFC3339)

		mgr.log.WithField("vmid", *shake.MachineId).WithField("message", *shake.Message).Info("Received agent handshake")
		err = m.Respond([]byte("OK"))
		if err != nil {
			mgr.log.WithError(err).Error("Failed to reply to agent handshake")
		}
	}
}

func logPublishSubject(namespace string, node string, workload string, vm string) string {
	// $NEX.logs.{namespace}.{node}.{workload name}.{vm}
	return fmt.Sprintf("%s.%s.%s.%s.%s", LogSubjectPrefix, namespace, node, workload, vm)
}
