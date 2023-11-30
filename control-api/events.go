package controlapi

const (
	AgentStartedEventType    = "agent_started"
	WorkloadStartedEventType = "workload_started"
	WorkloadStoppedEventType = "workload_stopped"
	AgentStoppedEventType    = "agent_stopped"
	NodeStartedEventType     = "node_started"
	NodeStoppedEventType     = "node_stopped"
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

type NodeStoppedEvent struct {
	Id       string `json:"id"`
	Graceful bool   `json:"graceful"`
}
