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
	agentWorkloadInfo, _ := w.LookupWorkload(agentId)
	if agentWorkloadInfo == nil {
		// got an event from a process that doesn't yet have a workload (deployment request) associated
		// with it
		return
	}

	evt.SetSource(fmt.Sprintf("%s-%s", w.publicKey, agentId))
	evt.SetExtension(controlapi.EventExtensionNamespace, *agentWorkloadInfo.Namespace)

	err := PublishCloudEvent(w.nc, *agentWorkloadInfo.Namespace, evt, w.log)
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

		if agentWorkloadInfo.IsEssential() && workloadStatus.Code != 0 {
			w.log.Debug("Essential workload stopped with non-zero exit code",
				slog.String("namespace", *agentWorkloadInfo.Namespace),
				slog.String("workload", *agentWorkloadInfo.WorkloadName),
				slog.String("workload_id", agentId),
				slog.String("workload_type", string(agentWorkloadInfo.WorkloadType)))

			if agentWorkloadInfo.RetryCount == nil {
				retryCount := uint(0)
				agentWorkloadInfo.RetryCount = &retryCount
			}

			*agentWorkloadInfo.RetryCount += 1

			retriedAt := time.Now().UTC()
			agentWorkloadInfo.RetriedAt = &retriedAt

			// generate a new uuid for this deploy request
			reqUUID, err := uuid.NewRandom()
			if err != nil {
				w.log.Error("Failed to generate unique identifier for deploy request", slog.Any("err", err))
				return
			}
			id := reqUUID.String()

			req, _ := json.Marshal(&controlapi.DeployRequest{
				Argv:               agentWorkloadInfo.Argv,
				Description:        agentWorkloadInfo.Description,
				Hash:               &agentWorkloadInfo.Hash,
				Environment:        agentWorkloadInfo.EncryptedEnvironment,
				Essential:          agentWorkloadInfo.Essential,
				HostServicesConfig: agentWorkloadInfo.HostServicesConfig,
				ID:                 &id,
				JsDomain:           agentWorkloadInfo.JsDomain,
				Location:           agentWorkloadInfo.Location,
				RetriedAt:          agentWorkloadInfo.RetriedAt,
				RetryCount:         agentWorkloadInfo.RetryCount,
				SenderPublicKey:    agentWorkloadInfo.SenderPublicKey,
				TriggerSubjects:    agentWorkloadInfo.TriggerSubjects,
				WorkloadJWT:        agentWorkloadInfo.WorkloadJWT,
				WorkloadName:       agentWorkloadInfo.WorkloadName,
				WorkloadType:       agentWorkloadInfo.WorkloadType,
			})

			nodeID := w.publicKey
			subject := fmt.Sprintf("%s.DEPLOY.%s.%s", controlapi.APIPrefix, *agentWorkloadInfo.Namespace, nodeID)
			_, err = w.nc.Request(subject, req, time.Millisecond*2500)
			if err != nil {
				w.log.Error("Failed to redeploy essential workload", slog.Any("err", err))
			}
		}
	}
}

func (w *WorkloadManager) agentLog(workloadID string, entry agentapi.LogEntry) {
	deployRequest, _ := w.LookupWorkload(workloadID)
	if deployRequest == nil {
		// we got a log from a process that has not yet received a deployment, so it doesn't have a
		// workload name or namespace
		return
	}

	bytes, err := json.Marshal(&emittedLog{
		Text:  entry.Text,
		Level: entry.Level,
		ID:    workloadID,
	})
	if err != nil {
		w.log.Error("Failed to marshal our own log entry", slog.Any("err", err))
		return
	}

	subject := logPublishSubject(*deployRequest.Namespace, w.publicKey, workloadID)
	_ = w.nc.Publish(subject, bytes)
}

func (w *WorkloadManager) publishFunctionExecFailed(workloadID string, tsub string, origErr error) error {
	deployRequest, err := w.LookupWorkload(workloadID)
	if err != nil {
		w.log.Error("Failed to look up workload", slog.String("workload_id", workloadID), slog.Any("error", err))
		return errors.New("function exec succeeded event was not published")
	}

	functionExecFailed := struct {
		ID        string `json:"workload_id"`
		Name      string `json:"workload_name"`
		Namespace string `json:"namespace"`
		Subject   string `json:"trigger_subject"`
		Error     string `json:"error"`
	}{
		ID:        workloadID,
		Name:      *deployRequest.WorkloadName,
		Namespace: *deployRequest.Namespace,
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

	err = PublishCloudEvent(w.nc, *deployRequest.Namespace, cloudevent, w.log)
	if err != nil {
		return err
	}

	emitLog := emittedLog{
		Text:  "Function execution failed",
		Level: slog.LevelError,
		ID:    workloadID,
	}
	logBytes, _ := json.Marshal(emitLog)

	subject := fmt.Sprintf("%s.%s.%s.%s", LogSubjectPrefix, *deployRequest.Namespace, w.publicKey, workloadID)
	err = w.nc.Publish(subject, logBytes)
	if err != nil {
		w.log.Error("Failed to publish function exec failed log", slog.Any("err", err))
	}

	return w.nc.Flush()
}

func (w *WorkloadManager) publishFunctionExecSucceeded(workloadID string, tsub string, elapsedNanos int64) error {
	deployRequest, err := w.LookupWorkload(workloadID)
	if err != nil {
		w.log.Error("Failed to look up workload", slog.String("workload_id", workloadID), slog.Any("error", err))
		return errors.New("function exec succeeded event was not published")
	}

	if deployRequest == nil {
		w.log.Warn("Tried to publish function exec succeeded event for non-existent workload", slog.String("workload_id", workloadID))
		return nil
	}

	functionExecPassed := struct {
		ID        string `json:"workload_id"`
		Name      string `json:"workload_name"`
		Namespace string `json:"namespace"`
		Subject   string `json:"trigger_subject"`
		Elapsed   int64  `json:"elapsed_nanos"`
	}{
		ID:        workloadID,
		Name:      *deployRequest.WorkloadName,
		Namespace: *deployRequest.Namespace,
		Subject:   tsub,
		Elapsed:   elapsedNanos,
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
		ID:    workloadID,
	}
	logBytes, _ := json.Marshal(emitLog)

	subject := fmt.Sprintf("%s.%s.%s.%s", LogSubjectPrefix, *deployRequest.Namespace, w.publicKey, workloadID)
	err = w.nc.Publish(subject, logBytes)
	if err != nil {
		w.log.Error("Failed to publish function exec passed log", slog.Any("err", err))
	}

	return w.nc.Flush()
}

// publish a workload undeployed event for the provided workload
func (w *WorkloadManager) publishWorkloadUndeployed(workloadID string) error {
	deployRequest, err := w.LookupWorkload(workloadID)
	if err != nil {
		w.log.Error("Failed to look up workload", slog.String("workload_id", workloadID), slog.Any("error", err))
		return errors.New("workload undeployed event was not published")
	}

	if deployRequest == nil {
		w.log.Warn("Tried to publish undeployed event for non-existent workload", slog.String("workload_id", workloadID))
		return errors.New("workload undeployed event was not published")
	}

	workloadName := strings.TrimSpace(*deployRequest.WorkloadName)
	if len(workloadName) > 0 {
		workloadUndeployed := struct {
			ID     string `json:"workload_id"`
			Name   string `json:"workload_name"`
			Reason string `json:"reason,omitempty"`
		}{
			ID:     workloadID,
			Name:   workloadName,
			Reason: "Workload undeploy requested",
		}

		cloudevent := cloudevents.NewEvent()
		cloudevent.SetSource(w.publicKey)
		cloudevent.SetID(uuid.NewString())
		cloudevent.SetTime(time.Now().UTC())
		cloudevent.SetType(agentapi.WorkloadUndeployedEventType)
		cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
		_ = cloudevent.SetData(workloadUndeployed)

		err := PublishCloudEvent(w.nc, *deployRequest.Namespace, cloudevent, w.log)
		if err != nil {
			return err
		}

		emitLog := emittedLog{
			Text:  "Workload undeployed",
			Level: slog.LevelDebug,
			ID:    workloadID,
		}
		logBytes, _ := json.Marshal(emitLog)

		subject := fmt.Sprintf("%s.%s.%s.%s", LogSubjectPrefix, *deployRequest.Namespace, w.publicKey, workloadID)
		err = w.nc.Publish(subject, logBytes)
		if err != nil {
			w.log.Error("Failed to publish workload undeployed event", slog.Any("err", err))
		}

		return w.nc.Flush()
	}

	return nil
}

func logPublishSubject(namespace, nodeID, workloadID string) string {
	// $NEX.logs.{namespace}.{node}.{workloadID}
	return fmt.Sprintf("%s.%s.%s.%s", LogSubjectPrefix, namespace, nodeID, workloadID)
}
