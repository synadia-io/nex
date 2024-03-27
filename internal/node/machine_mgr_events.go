package nexnode

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/google/uuid"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	controlapi "github.com/synadia-io/nex/internal/control-api"
)

func (m *MachineManager) publishFunctionExecSucceeded(vm *runningFirecracker, tsub string, elapsedNanos int64) error {
	functionExecPassed := struct {
		Name      string `json:"workload_name"`
		Subject   string `json:"trigger_subject"`
		Elapsed   int64  `json:"elapsed_nanos"`
		Namespace string `json:"namespace"`
	}{
		Name:      *vm.deployRequest.WorkloadName,
		Subject:   tsub,
		Elapsed:   elapsedNanos,
		Namespace: vm.namespace,
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(m.node.publicKey)
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType(agentapi.FunctionExecutionSucceededType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(functionExecPassed)

	err := PublishCloudEvent(m.node.nc, vm.namespace, cloudevent, m.log)
	if err != nil {
		return err
	}

	emitLog := emittedLog{
		Text:      fmt.Sprintf("Function %s execution succeeded (%dns)", functionExecPassed.Name, functionExecPassed.Elapsed),
		Level:     slog.LevelDebug,
		MachineId: vm.vmmID,
	}
	logBytes, _ := json.Marshal(emitLog)

	subject := fmt.Sprintf("%s.%s.%s.%s.%s", LogSubjectPrefix, vm.namespace, m.node.publicKey, *vm.deployRequest.WorkloadName, vm.vmmID)
	err = m.node.nc.Publish(subject, logBytes)
	if err != nil {
		m.log.Error("Failed to publish function exec passed log", slog.Any("err", err))
	}

	return m.node.nc.Flush()
}

func (m *MachineManager) publishFunctionExecFailed(vm *runningFirecracker, workload string, tsub string, origErr error) error {

	functionExecFailed := struct {
		Name      string `json:"workload_name"`
		Subject   string `json:"trigger_subject"`
		Namespace string `json:"namespace"`
		Error     string `json:"error"`
	}{
		Name:      workload,
		Namespace: vm.namespace,
		Subject:   tsub,
		Error:     origErr.Error(),
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(m.node.publicKey)
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType(agentapi.FunctionExecutionFailedType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(functionExecFailed)

	err := PublishCloudEvent(m.node.nc, vm.namespace, cloudevent, m.log)
	if err != nil {
		return err
	}

	emitLog := emittedLog{
		Text:      "Function execution failed",
		Level:     slog.LevelError,
		MachineId: vm.vmmID,
	}
	logBytes, _ := json.Marshal(emitLog)

	subject := fmt.Sprintf("%s.%s.%s.%s.%s", LogSubjectPrefix, vm.namespace, m.node.publicKey, *vm.deployRequest.WorkloadName, vm.vmmID)
	err = m.node.nc.Publish(subject, logBytes)
	if err != nil {
		m.log.Error("Failed to publish function exec failed log", slog.Any("err", err))
	}

	return m.node.nc.Flush()
}

// publishMachineStopped writes a workload stopped event for the provided firecracker VM
func (m *MachineManager) publishMachineStopped(vm *runningFirecracker) error {
	if vm.deployRequest == nil {
		return errors.New("machine stopped event was not published")
	}

	workloadName := strings.TrimSpace(vm.deployRequest.DecodedClaims.Subject)
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
		cloudevent.SetSource(m.node.publicKey)
		cloudevent.SetID(uuid.NewString())
		cloudevent.SetTime(time.Now().UTC())
		cloudevent.SetType(agentapi.WorkloadStoppedEventType)
		cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
		_ = cloudevent.SetData(workloadStopped)

		err := PublishCloudEvent(m.node.nc, vm.namespace, cloudevent, m.log)
		if err != nil {
			return err
		}

		emitLog := emittedLog{
			Text:      "Workload stopped",
			Level:     slog.LevelDebug,
			MachineId: vm.vmmID,
		}
		logBytes, _ := json.Marshal(emitLog)

		subject := fmt.Sprintf("%s.%s.%s.%s.%s", LogSubjectPrefix, vm.namespace, m.node.publicKey, workloadName, vm.vmmID)
		err = m.node.nc.Publish(subject, logBytes)
		if err != nil {
			m.log.Error("Failed to publish machine stopped event", slog.Any("err", err))
		}

		return m.node.nc.Flush()
	}

	return nil
}

// // Called when the node server gets a log entry via internal NATS. Used to
// // package and re-mit with additional metadata on $NEX.logs...
// func (m *MachineManager) handleAgentLog(msg *nats.Msg) {
// 	tokens := strings.Split(msg.Subject, ".")
// 	vmID := tokens[1]

// 	vm, ok := m.allVMs[vmID]
// 	if !ok {
// 		m.log.Warn("Received a log message from an unknown VM.")
// 		return
// 	}

// 	var logentry agentapi.LogEntry
// 	err := json.Unmarshal(msg.Data, &logentry)
// 	if err != nil {
// 		m.log.Error("Failed to unmarshal log entry from agent", slog.Any("err", err))
// 		return
// 	}

// 	m.log.Debug("Received agent log", slog.String("vmid", vmID), slog.String("log", logentry.Text))

// 	bytes, err := json.Marshal(&emittedLog{
// 		Text:      logentry.Text,
// 		Level:     slog.Level(logentry.Level),
// 		MachineId: vmID,
// 	})
// 	if err != nil {
// 		m.log.Error("Failed to marshal our own log entry", slog.Any("err", err))
// 		return
// 	}

// 	var workload *string
// 	if vm.deployRequest != nil {
// 		workload = vm.deployRequest.WorkloadName
// 	}

// 	subject := logPublishSubject(vm.namespace, m.node.publicKey, vmID, workload)
// 	_ = m.nc.Publish(subject, bytes)
// }

func (m *MachineManager) agentLog(agentId string, entry agentapi.LogEntry) {
	vm, ok := m.allVMs[agentId]
	if !ok {
		m.log.Warn("Received a log message from an unknown VM.")
		return
	}

	bytes, err := json.Marshal(&emittedLog{
		Text:      entry.Text,
		Level:     slog.Level(entry.Level),
		MachineId: agentId,
	})
	if err != nil {
		m.log.Error("Failed to marshal our own log entry", slog.Any("err", err))
		return
	}

	var workload *string
	if vm.deployRequest != nil {
		workload = vm.deployRequest.WorkloadName
	}

	subject := logPublishSubject(vm.namespace, m.node.publicKey, agentId, workload)
	_ = m.node.nc.Publish(subject, bytes)

}
func (m *MachineManager) agentEvent(agentId string, evt cloudevents.Event) {
	vm, ok := m.allVMs[agentId]
	if !ok {
		m.log.Warn("Received an event from a VM we don't know about. Rejecting.")
		return
	}

	err := PublishCloudEvent(m.node.nc, vm.namespace, evt, m.log)
	if err != nil {
		m.log.Error("Failed to publish cloudevent", slog.Any("err", err))
		return
	}

	if evt.Type() == agentapi.WorkloadStoppedEventType {
		_ = m.StopMachine(agentId, false)

		evtData, err := evt.DataBytes()
		if err != nil {
			m.log.Error("Failed to read cloudevent data", slog.Any("err", err))
			return
		}

		var workloadStatus *agentapi.WorkloadStatusEvent
		err = json.Unmarshal(evtData, &workloadStatus)
		if err != nil {
			m.log.Error("Failed to unmarshal workload status from cloudevent data", slog.Any("err", err))
			return
		}

		if vm.isEssential() && workloadStatus.Code != 0 {
			m.log.Debug("Essential workload stopped with non-zero exit code",
				slog.String("vmid", agentId),
				slog.String("namespace", *vm.deployRequest.Namespace),
				slog.String("workload", *vm.deployRequest.WorkloadName),
				slog.String("workload_type", *vm.deployRequest.WorkloadType))

			if vm.deployRequest.RetryCount == nil {
				retryCount := uint(0)
				vm.deployRequest.RetryCount = &retryCount
			}

			*vm.deployRequest.RetryCount += 1

			retriedAt := time.Now().UTC()
			vm.deployRequest.RetriedAt = &retriedAt

			req, _ := json.Marshal(&controlapi.DeployRequest{
				Argv:            vm.deployRequest.Argv,
				Description:     vm.deployRequest.Description,
				WorkloadType:    vm.deployRequest.WorkloadType,
				Location:        vm.deployRequest.Location,
				WorkloadJwt:     vm.deployRequest.WorkloadJwt,
				Environment:     vm.deployRequest.EncryptedEnvironment,
				Essential:       vm.deployRequest.Essential,
				RetriedAt:       vm.deployRequest.RetriedAt,
				RetryCount:      vm.deployRequest.RetryCount,
				SenderPublicKey: vm.deployRequest.SenderPublicKey,
				TargetNode:      vm.deployRequest.TargetNode,
				TriggerSubjects: vm.deployRequest.TriggerSubjects,
				JsDomain:        vm.deployRequest.JsDomain,
			})

			nodeID, _ := m.kp.PublicKey()
			subject := fmt.Sprintf("%s.DEPLOY.%s.%s", controlapi.APIPrefix, vm.namespace, nodeID)
			_, err = m.node.nc.Request(subject, req, time.Millisecond*2500)
			if err != nil {
				m.log.Error("Failed to redeploy essential workload", slog.Any("err", err))
			}
		}
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
