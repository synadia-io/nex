package agentapi

import (
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/google/uuid"
)

const (
	AgentStartedEventType    = "agent_started"
	AgentStoppedEventType    = "agent_stopped"
	WorkloadStartedEventType = "workload_started" // FIXME-- should this be WorkloadDeployed?
	WorkloadStoppedEventType = "workload_stopped" // FIXME-- should this be in addition to WorkloadUndeployed (likely yes, in case of something bad happening...)
	// FIXME-- where is WorkloadDeployedEventType? (likely just need to rename WorkloadStartedEventType -> WorkloadDeployedEventType)
	// FIXME-- where is WorkloadStoppedEventType?
)

type AgentStartedEvent struct {
	AgentVersion string `json:"agent_version"`
}

type WorkloadStatusEvent struct {
	WorkloadName string `json:"workload_name"`
	Code         int    `json:"code"`
	Message      string `json:"message,omitempty"`
}

type AgentStoppedEvent struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
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
