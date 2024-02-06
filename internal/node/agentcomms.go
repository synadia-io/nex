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
	vmId := tokens[1]

	vm, ok := mgr.allVMs[vmId]
	if !ok {
		mgr.log.Warn("Received a log message from an unknown VM.")
		return
	}

	if vm.deployRequest == nil {
		mgr.log.Warn("Received a log message from a VM without a reference to its deploy request; attempting redelivery in 50ms.")
		_ = m.NakWithDelay(time.Millisecond * 50)
		return
	}

	var logentry agentapi.LogEntry
	err := json.Unmarshal(m.Data, &logentry)
	if err != nil {
		mgr.log.Error("Failed to unmarshal log entry from agent", slog.Any("err", err))
		return
	}

	outLog := emittedLog{
		Text:      logentry.Text,
		Level:     slog.Level(logentry.Level),
		MachineId: vmId,
	}

	bytes, err := json.Marshal(outLog)
	if err != nil {
		mgr.log.Error("Failed to marshal our own log entry", slog.Any("err", err))
		return
	}

	subject := logPublishSubject(vm.namespace, mgr.publicKey, *vm.deployRequest.WorkloadName, vmId)
	_ = mgr.nc.Publish(subject, bytes)
}

// Called when the node server gets an event from the nex agent inside firecracker. The data here is already a fully formed
// cloud event, so all we need to do is unmarshal it, get some metadata, and then republish on $NEX.events...
func (mgr *MachineManager) handleAgentEvent(m *nats.Msg) {
	// agentint.{vmid}.events.{type}
	tokens := strings.Split(m.Subject, ".")
	vmId := tokens[1]

	vm, ok := mgr.allVMs[vmId]
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
	mgr.log.Info("Received agent event", slog.String("vmid", vmId), slog.String("type", evt.Type()))

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
		mgr.log.Error("Failed to handle agent handshake", slog.String("vmid", *shake.MachineId), slog.String("message", *shake.Message))
		return
	}

	now := time.Now().UTC()
	mgr.handshakes[*shake.MachineId] = now.Format(time.RFC3339)

	mgr.log.Info("Received agent handshake", slog.String("vmid", *shake.MachineId), slog.String("message", *shake.Message))
	err = m.Respond([]byte("OK"))
	if err != nil {
		mgr.log.Error("Failed to reply to agent handshake", slog.Any("err", err))
	}
}

func logPublishSubject(namespace string, node string, workload string, vm string) string {
	// $NEX.logs.{namespace}.{node}.{workload name}.{vm}
	return fmt.Sprintf("%s.%s.%s.%s.%s", LogSubjectPrefix, namespace, node, workload, vm)
}

