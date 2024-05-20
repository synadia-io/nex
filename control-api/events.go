package controlapi

const (
	AgentStartedEventType    = "agent_started"
	AgentStoppedEventType    = "agent_stopped"
	NodeStartedEventType     = "node_started"
	NodeStoppedEventType     = "node_stopped"
	LameDuckEnteredEventType = "node_entered_lameduck"
	HeartbeatEventType       = "heartbeat"
	WorkloadStartedEventType = "workload_started" // FIXME-- should this be WorkloadDeployed?
	WorkloadStoppedEventType = "workload_stopped" // FIXME-- should this be in addition to WorkloadUndeployed (likely yes, in case of something bad happening...)
	// FIXME-- where is WorkloadDeployedEventType? (likely just need to rename WorkloadStartedEventType -> WorkloadDeployedEventType)
	// FIXME-- where is WorkloadStoppedEventType?
)

type AgentStartedEvent struct {
	AgentVersion string `json:"agent_version"`
}

type WorkloadStartedEvent struct {
	Name       string `json:"workload_name"`
	TotalBytes int    `json:"total_bytes"`
}

type WorkloadStoppedEvent struct {
	Name    string `json:"workload_name"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type AgentStoppedEvent struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type NodeStartedEvent struct {
	Version string `json:"version"`
	Id      string `json:"id"`
}

type LameDuckEnteredEvent struct {
	Version string `json:"version"`
	Id      string `json:"id"`
}

type NodeStoppedEvent struct {
	Id       string `json:"id"`
	Graceful bool   `json:"graceful"`
}

// TODO: remove omitempty in next version bump
type HeartbeatEvent struct {
	Version         string            `json:"version"`
	NodeId          string            `json:"node_id"`
	Nexus           string            `json:"nexus,omitempty"`
	Uptime          string            `json:"uptime"`
	Tags            map[string]string `json:"tags,omitempty"`
	RunningMachines int               `json:"running_machines"`
}
