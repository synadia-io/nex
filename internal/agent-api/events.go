package agentapi

import (
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/google/uuid"
)

const (
	AgentStartedEventType          = "agent_started"
	AgentStoppedEventType          = "agent_stopped"
	FunctionExecutionFailedType    = "function_exec_failed"
	FunctionExecutionSucceededType = "function_exec_succeeded"
	WorkloadDeployedEventType      = "workload_deployed"
	WorkloadUndeployedEventType    = "workload_undeployed"
)

type AgentStartedEvent struct {
	AgentVersion string `json:"agent_version"`
}

type WorkloadStatusEvent struct {
	Code         int    `json:"code"`
	Message      string `json:"message,omitempty"`
	WorkloadID   string `json:"workload_id"`
	WorkloadName string `json:"workload_name"`
}

type AgentStoppedEvent struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewAgentEvent(sourceId string, eventType string, event interface{}) cloudevents.Event {
	cloudevent := cloudevents.NewEvent()

	cloudevent.SetSource(sourceId)
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType(eventType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(event)

	return cloudevent
}
