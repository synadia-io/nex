package nexnode

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

// Called when the node server gets a log entry via internal NATS. Used to
// package and re-mit with additional metadata on $NEX.logs...
func (mgr *MachineManager) handleAgentLog(m *nats.Msg) {
	tokens := strings.Split(m.Subject, ".")
	vmID := tokens[1]

	vm, ok := mgr.allVMs[vmID]
	if !ok {
		mgr.log.Warn("Received a log message from an unknown VM.")
		return
	}

	var logentry agentapi.LogEntry
	err := json.Unmarshal(m.Data, &logentry)
	if err != nil {
		mgr.log.Error("Failed to unmarshal log entry from agent", slog.Any("err", err))
		return
	}

	mgr.log.Debug("Received agent log", slog.String("vmid", vmID), slog.String("log", logentry.Text))

	bytes, err := json.Marshal(&emittedLog{
		Text:      logentry.Text,
		Level:     slog.Level(logentry.Level),
		MachineId: vmID,
	})
	if err != nil {
		mgr.log.Error("Failed to marshal our own log entry", slog.Any("err", err))
		return
	}

	var workload *string
	if vm.deployRequest != nil {
		workload = vm.deployRequest.WorkloadName
	}

	subject := logPublishSubject(vm.namespace, mgr.publicKey, vmID, workload)
	_ = mgr.nc.Publish(subject, bytes)
}

// Called when the node server gets an event from the nex agent inside firecracker. The data here is already a fully formed
// cloud event, so all we need to do is unmarshal it, get some metadata, and then republish on $NEX.events...
func (mgr *MachineManager) handleAgentEvent(m *nats.Msg) {
	// agentint.{vmid}.events.{type}
	tokens := strings.Split(m.Subject, ".")
	vmID := tokens[1]

	vm, ok := mgr.allVMs[vmID]
	if !ok {
		mgr.log.Warn("Received an event from a VM we don't know about. Rejecting.")
		return
	}

	var evt cloudevents.Event
	err := json.Unmarshal(m.Data, &evt)
	if err != nil {
		mgr.log.Error("Failed to deserialize cloudevent from agent", slog.Any("err", err))
		return
	}

	mgr.log.Info("Received agent event", slog.String("vmid", vmID), slog.String("type", evt.Type()))

	err = PublishCloudEvent(mgr.nc, vm.namespace, evt, mgr.log)
	if err != nil {
		mgr.log.Error("Failed to publish cloudevent", slog.Any("err", err))
		return
	}

	if evt.Type() == agentapi.WorkloadStoppedEventType {
		vm.shutdown()
	}
}

// This handshake uses the request pattern to force a full round trip to ensure connectivity is working properly as
// fire-and-forget publishes from inside the firecracker VM could potentially be lost
func (mgr *MachineManager) handleHandshake(m *nats.Msg) {
	var shake agentapi.HandshakeRequest
	err := json.Unmarshal(m.Data, &shake)
	if err != nil {
		mgr.log.Error("Failed to handle agent handshake", slog.String("vmid", *shake.MachineID), slog.String("message", *shake.Message))
		return
	}

	now := time.Now().UTC()
	mgr.handshakes[*shake.MachineID] = now.Format(time.RFC3339)

	mgr.log.Info("Received agent handshake", slog.String("vmid", *shake.MachineID), slog.String("message", *shake.Message))
	err = m.Respond([]byte("OK"))
	if err != nil {
		mgr.log.Error("Failed to reply to agent handshake", slog.Any("err", err))
	}
}

func logPublishSubject(namespace string, node string, vm string, workload *string) string {
	// $NEX.logs.{namespace}.{node}.{vm}[.{workload name}]
	subject := fmt.Sprintf("%s.%s.%s.%s", LogSubjectPrefix, namespace, node, vm)
	if workload != nil {
		subject = fmt.Sprintf("%s.%s", subject, *workload)
	}

	return subject
}
