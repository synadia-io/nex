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
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

func (w *WorkloadManager) agentEvent(agentId string, evt cloudevents.Event) {
	deployRequest, _ := w.LookupWorkload(agentId)
	if deployRequest == nil {
		// got an event from a process that doesn't yet have a workload (deployment request) associated
		// with it
		return
	}
	evt.SetSource(fmt.Sprintf("%s-%s", *deployRequest.TargetNode, agentId))
	evt.SetExtension(controlapi.EventExtensionNamespace, *deployRequest.Namespace)

	err := PublishCloudEvent(w.nc, *deployRequest.Namespace, evt, w.log)
	if err != nil {
		w.log.Error("Failed to publish cloudevent", slog.Any("err", err))
		return
	}

	if evt.Type() == agentapi.WorkloadUndeployedEventType {
		_ = w.StopWorkload(agentId, false)

		evtData, err := evt.DataBytes()
		if err != nil {
			w.log.Error("Failed to read cloudevent data", slog.Any("err", err))
			return
		}

		var workloadStatus *agentapi.WorkloadStatusEvent
		err = json.Unmarshal(evtData, &workloadStatus)
		if err != nil {
			w.log.Error("Failed to unmarshal workload status from cloudevent data", slog.Any("err", err))
			return
		}

		if deployRequest.IsEssential() && workloadStatus.Code != 0 {
			w.log.Debug("Essential workload stopped with non-zero exit code",
				slog.String("namespace", *deployRequest.Namespace),
				slog.String("workload", *deployRequest.WorkloadName),
				slog.String("workload_id", agentId),
				slog.String("workload_type", string(deployRequest.WorkloadType)))

			if deployRequest.RetryCount == nil {
				retryCount := uint(0)
				deployRequest.RetryCount = &retryCount
			}

			*deployRequest.RetryCount += 1

			retriedAt := time.Now().UTC()
			deployRequest.RetriedAt = &retriedAt

			req, _ := json.Marshal(&controlapi.DeployRequest{
				Argv:            deployRequest.Argv,
				Description:     deployRequest.Description,
				Environment:     deployRequest.EncryptedEnvironment,
				Essential:       deployRequest.Essential,
				JsDomain:        deployRequest.JsDomain,
				Location:        deployRequest.Location,
				RetriedAt:       deployRequest.RetriedAt,
				RetryCount:      deployRequest.RetryCount,
				SenderPublicKey: deployRequest.SenderPublicKey,
				TargetNode:      deployRequest.TargetNode,
				TriggerSubjects: deployRequest.TriggerSubjects,
				WorkloadJwt:     deployRequest.WorkloadJwt,
				WorkloadType:    deployRequest.WorkloadType,
			})

			nodeID := w.publicKey
			subject := fmt.Sprintf("%s.DEPLOY.%s.%s", controlapi.APIPrefix, *deployRequest.Namespace, nodeID)
			_, err = w.nc.Request(subject, req, time.Millisecond*2500)
			if err != nil {
				w.log.Error("Failed to redeploy essential workload", slog.Any("err", err))
			}
		}
	}
}

func (w *WorkloadManager) agentLog(workloadId string, entry agentapi.LogEntry) {
	deployRequest, _ := w.LookupWorkload(workloadId)
	if deployRequest == nil {
		// we got a log from a process that has not yet received a deployment, so it doesn't have a
		// workload name or namespace
		return
	}

	bytes, err := json.Marshal(&emittedLog{
		Text:  entry.Text,
		Level: entry.Level,
		ID:    workloadId,
	})
	if err != nil {
		w.log.Error("Failed to marshal our own log entry", slog.Any("err", err))
		return
	}

	subject := logPublishSubject(*deployRequest.Namespace, w.publicKey, workloadId, deployRequest.WorkloadName)
	_ = w.nc.Publish(subject, bytes)
}

func (w *WorkloadManager) publishFunctionExecFailed(workloadId string, workloadName string, namespace string, tsub string, origErr error) error {
	functionExecFailed := struct {
		Name      string `json:"workload_name"`
		Subject   string `json:"trigger_subject"`
		Namespace string `json:"namespace"`
		Error     string `json:"error"`
	}{
		Name:      workloadName,
		Namespace: namespace,
		Subject:   tsub,
		Error:     origErr.Error(),
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(w.publicKey)
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType(agentapi.FunctionExecutionFailedType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(functionExecFailed)

	err := PublishCloudEvent(w.nc, namespace, cloudevent, w.log)
	if err != nil {
		return err
	}

	emitLog := emittedLog{
		Text:  "Function execution failed",
		Level: slog.LevelError,
		ID:    workloadId,
	}
	logBytes, _ := json.Marshal(emitLog)

	subject := fmt.Sprintf("%s.%s.%s.%s.%s", LogSubjectPrefix, namespace, w.publicKey, workloadName, workloadId)
	err = w.nc.Publish(subject, logBytes)
	if err != nil {
		w.log.Error("Failed to publish function exec failed log", slog.Any("err", err))
	}

	return w.nc.Flush()
}

func (w *WorkloadManager) publishFunctionExecSucceeded(workloadId string, tsub string, elapsedNanos int64) error {
	deployRequest, err := w.LookupWorkload(workloadId)
	if err != nil {
		w.log.Error("Failed to look up workload", slog.String("workload_id", workloadId), slog.Any("error", err))
		return errors.New("function exec succeeded event was not published")
	}

	if deployRequest == nil {
		w.log.Warn("Tried to publish function exec succeeded event for non-existent workload", slog.String("workload_id", workloadId))
		return nil
	}

	functionExecPassed := struct {
		Name      string `json:"workload_name"`
		Subject   string `json:"trigger_subject"`
		Elapsed   int64  `json:"elapsed_nanos"`
		Namespace string `json:"namespace"`
	}{
		Name:      *deployRequest.WorkloadName,
		Subject:   tsub,
		Elapsed:   elapsedNanos,
		Namespace: *deployRequest.Namespace,
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(w.publicKey)
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType(agentapi.FunctionExecutionSucceededType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(functionExecPassed)

	err = PublishCloudEvent(w.nc, *deployRequest.Namespace, cloudevent, w.log)
	if err != nil {
		return err
	}

	emitLog := emittedLog{
		Text:  fmt.Sprintf("Function %s execution succeeded (%dns)", functionExecPassed.Name, functionExecPassed.Elapsed),
		Level: slog.LevelDebug,
		ID:    workloadId,
	}
	logBytes, _ := json.Marshal(emitLog)

	subject := fmt.Sprintf("%s.%s.%s.%s.%s", LogSubjectPrefix, *deployRequest.Namespace, w.publicKey, *deployRequest.WorkloadName, workloadId)
	err = w.nc.Publish(subject, logBytes)
	if err != nil {
		w.log.Error("Failed to publish function exec passed log", slog.Any("err", err))
	}

	return w.nc.Flush()
}

// publishWorkloadStopped writes a workload stopped event for the provided workload
func (w *WorkloadManager) publishWorkloadStopped(workloadId string) error {
	deployRequest, err := w.LookupWorkload(workloadId)
	if err != nil {
		w.log.Error("Failed to look up workload", slog.String("workload_id", workloadId), slog.Any("error", err))
		return errors.New("workload stopped event was not published")
	}

	if deployRequest == nil {
		w.log.Warn("Tried to publish stopped event for non-existent workload", slog.String("workload_id", workloadId))
		return errors.New("workload stopped event was not published")
	}

	workloadName := strings.TrimSpace(deployRequest.DecodedClaims.Subject)
	if len(workloadName) > 0 {
		workloadStopped := struct {
			Name   string `json:"name"`
			Reason string `json:"reason,omitempty"`
			VmId   string `json:"vmid"`
		}{
			Name:   workloadName,
			Reason: "Workload shutdown requested",
			VmId:   workloadId,
		}

		cloudevent := cloudevents.NewEvent()
		cloudevent.SetSource(w.publicKey)
		cloudevent.SetID(uuid.NewString())
		cloudevent.SetTime(time.Now().UTC())
		cloudevent.SetType(agentapi.WorkloadUndeployedEventType)
		cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
		_ = cloudevent.SetData(workloadStopped)

		err := PublishCloudEvent(w.nc, *deployRequest.Namespace, cloudevent, w.log)
		if err != nil {
			return err
		}

		emitLog := emittedLog{
			Text:  "Workload stopped",
			Level: slog.LevelDebug,
			ID:    workloadId,
		}
		logBytes, _ := json.Marshal(emitLog)

		subject := fmt.Sprintf("%s.%s.%s.%s.%s", LogSubjectPrefix, *deployRequest.Namespace, w.publicKey, workloadName, workloadId)
		err = w.nc.Publish(subject, logBytes)
		if err != nil {
			w.log.Error("Failed to publish machine stopped event", slog.Any("err", err))
		}

		return w.nc.Flush()
	}

	return nil
}

func logPublishSubject(namespace string, node string, vm string, workload *string) string {
	// $NEX.logs.{namespace}.{node}.{vm}[.{workload name}]
	subject := fmt.Sprintf("%s.%s.%s.%s", LogSubjectPrefix, namespace, node, vm)
	if workload != nil {
		subject = fmt.Sprintf("%s.%s", subject, *workload)
	}

	return subject
}
