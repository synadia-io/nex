package agentapi

import (
	"errors"
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

	TmpFilename *string `json:"-"`
	VmID        string  `json:"-"`
}

type WorkRequest struct {
	Environment  map[string]string `json:"environment"`
	Hash         *string           `json:"hash,omitempty"`
	TotalBytes   *int32            `json:"total_bytes,omitempty"`
	WorkloadName *string           `json:"workload_name,omitempty"`
	WorkloadType *string           `json:"workload_type,omitempty"`

	Stderr      io.Writer `json:"-"`
	Stdout      io.Writer `json:"-"`
	TmpFilename *string   `json:"-"`

	Errors []error `json:"errors,omitempty"`
}

func (w *WorkRequest) Validate() bool {
	w.Errors = make([]error, 0)

	if w.WorkloadName == nil {
		w.Errors = append(w.Errors, errors.New("workload name is required"))
	}

	if w.Hash == nil {
		w.Errors = append(w.Errors, errors.New("hash is required"))
	}

	if w.TotalBytes == nil {
		w.Errors = append(w.Errors, errors.New("total bytes is required"))
	}

	if w.WorkloadType == nil {
		w.Errors = append(w.Errors, errors.New("workload type is required"))
	}

	return len(w.Errors) == 0
}

type WorkResponse struct {
	Accepted bool    `json:"accepted"`
	Message  *string `json:"message"`
}

type HandshakeRequest struct {
	MachineId *string   `json:"machine_id"`
	StartTime time.Time `json:"start_time"`
	Message   *string   `json:"message,omitempty"`
}

type MachineMetadata struct {
	VmId            *string `json:"vmid"`
	NodeNatsAddress *string `json:"node_address"`
	NodePort        *int    `json:"node_port"`
	Message         *string `json:"message"`

	Errors []error `json:"errors,omitempty"`
}

func (m *MachineMetadata) Validate() bool {
	m.Errors = make([]error, 0)

	if m.VmId == nil {
		m.Errors = append(m.Errors, errors.New("vm id is required"))
	}

	if m.NodeNatsAddress == nil {
		m.Errors = append(m.Errors, errors.New("node NATS address is required"))
	}

	if m.NodePort == nil {
		m.Errors = append(m.Errors, errors.New("node port is required"))
	}

	return len(m.Errors) == 0
}

type LogEntry struct {
	Source string   `json:"source,omitempty"`
	Level  LogLevel `json:"level,omitempty"`
	Text   string   `json:"text,omitempty"`
}

type LogLevel int32
