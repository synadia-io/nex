package agentapi

import (
	"io"
	"time"
)

// WorkloadCacheBucket is an internal, non-public bucket for sharing files between host and agent
const WorkloadCacheBucket = "NEXCACHE"

// DefaultRunloopSleepTimeoutMillis default number of milliseconds to sleep during execution runloops
const DefaultRunloopSleepTimeoutMillis = 25

// ExecutionProviderParams parameters for initializing a specific execution provider
type ExecutionProviderParams struct {
	WorkRequest

	// Fail channel receives bool upon command failing to start
	Fail chan bool `json:"-"`

	// Run channel receives bool upon command successfully starting
	Run chan bool `json:"-"`

	// Exit channel receives int exit code upon command exit
	Exit chan int `json:"-"`

	Stderr io.Writer `json:"-"`
	Stdout io.Writer `json:"-"`

	TmpFilename     string           `json:"-"`
	VmID            string           `json:"-"`
	MachineMetadata *MachineMetadata `json:"-"`
}

type WorkRequest struct {
	WorkloadName string            `json:"workload_name"`
	Hash         string            `json:"hash"`
	TotalBytes   int32             `json:"total_bytes"`
	Environment  map[string]string `json:"environment"`
	WorkloadType string            `json:"workload_type,omitempty"`

	Stderr      io.Writer `json:"-"`
	Stdout      io.Writer `json:"-"`
	TmpFilename string    `json:"-"`
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
