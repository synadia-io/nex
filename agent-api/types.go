package agentapi

import "time"

const (
	// Internal bucket for sharing files between host and agent. This is not a public bucket
	WorkloadCacheBucket = "NEXCACHE"
)

type WorkRequest struct {
	WorkloadName string            `json:"workload_name"`
	Hash         string            `json:"hash"`
	TotalBytes   int               `json:"total_bytes"`
	Environment  map[string]string `json:"environment"`
	WorkloadType string            `json:"workload_type,omitempty"`
}

type WorkResponse struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message"`
}

type AdvertiseMessage struct {
	MachineId string    `json:"machine_id"`
	StartTime time.Time `json:"start_time"`
	Message   string    `json:"message,omitempty"`
}

type MachineMetadata struct {
	VmId            string `json:"vmid"`
	NodeNatsAddress string `json:"node_address"`
	NodePort        int    `json:"node_port"`
	Message         string `json:"message"`
}

type LogEntry struct {
	Source string   `json:"source,omitempty"`
	Level  LogLevel `json:"level,omitempty"`
	Text   string   `json:"text,omitempty"`
}

type LogLevel int32
