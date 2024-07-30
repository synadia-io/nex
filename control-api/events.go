package controlapi

const (
	AgentStartedEventType       = "agent_started"
	AgentStoppedEventType       = "agent_stopped"
	NodeStartedEventType        = "node_started"
	NodeStoppedEventType        = "node_stopped"
	LameDuckEnteredEventType    = "node_entered_lameduck"
	HeartbeatEventType          = "heartbeat"
	WorkloadDeployedEventType   = "workload_deployed"
	WorkloadUndeployedEventType = "workload_undeployed"
)

type AgentStartedEvent struct {
	AgentVersion string `json:"agent_version"`
}

type WorkloadDeployedEvent struct {
	ID         string `json:"workload_id"`
	Name       string `json:"workload_name"`
	Essential  bool   `json:"essential"`
	TotalBytes int    `json:"total_bytes"`
}

type WorkloadUndeployedEvent struct {
	ID      string `json:"workload_id"`
	Name    string `json:"workload_name"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type AgentStoppedEvent struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type NodeStartedEvent struct {
	Version string            `json:"version"`
	ID      string            `json:"id"`
	Tags    map[string]string `json:"tags,omitempty"`
}

type LameDuckEnteredEvent struct {
	Version string `json:"version"`
	ID      string `json:"id"`
}

type NodeStoppedEvent struct {
	ID       string `json:"id"`
	Graceful bool   `json:"graceful"`
}

type HeartbeatEvent struct {
	AllowDuplicateWorkloads *bool             `json:"allow_duplicate_workloads"`
	Nexus                   string            `json:"nexus,omitempty"`
	NodeID                  string            `json:"node_id"`
	RunningMachines         int               `json:"running_machines"`
	Tags                    map[string]string `json:"tags,omitempty"`
	Uptime                  string            `json:"uptime"`
	Version                 string            `json:"version"`
}
