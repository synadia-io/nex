package agentapi

import (
	"errors"
	"io"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

// Agent handshake subject
const NexAgentSubjectHandshake = "agentint.handshake"

// Executable Linkable Format execution provider
const NexExecutionProviderELF = "elf"

// V8 execution provider
const NexExecutionProviderV8 = "v8"

// OCI execution provider
const NexExecutionProviderOCI = "oci"

// Wasm execution provider
const NexExecutionProviderWasm = "wasm"

// Name of the internal, non-public bucket for sharing files between host and agent
const WorkloadCacheBucket = "NEXCACHE"

// DefaultRunloopSleepTimeoutMillis default number of milliseconds to sleep during execution runloops
const DefaultRunloopSleepTimeoutMillis = 25

// ExecutionProviderParams parameters for initializing a specific execution provider
type ExecutionProviderParams struct {
	DeployRequest
	TriggerSubjects []string `json:"trigger_subjects"`

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

	// NATS connection which be injected into the execution provider
	NATSConn *nats.Conn `json:"-"`
}

// FIXME? DeployRequest -> DeployRequest?
type DeployRequest struct {
	Environment     map[string]string `json:"environment"`
	Hash            string            `json:"hash,omitempty"`
	TotalBytes      int64             `json:"total_bytes,omitempty"`
	TriggerSubjects []string          `json:"trigger_subjects"`
	WorkloadName    *string           `json:"workload_name,omitempty"`
	WorkloadType    *string           `json:"workload_type,omitempty"`
	Namespace       *string           `json:"namespace,omitempty"`

	Stderr      io.Writer `json:"-"`
	Stdout      io.Writer `json:"-"`
	TmpFilename *string   `json:"-"`

	Errors []error `json:"errors,omitempty"`
}

func (w *DeployRequest) Validate() bool {
	w.Errors = make([]error, 0)

	if w.WorkloadName == nil {
		w.Errors = append(w.Errors, errors.New("workload name is required"))
	}

	// FIXME-- this should be provided in the request
	// if w.Hash == nil {
	// 	w.Errors = append(w.Errors, errors.New("hash is required"))
	// }

	// FIXME-- this should be provided in the request
	// if w.TotalBytes == nil {
	// 	w.Errors = append(w.Errors, errors.New("total bytes is required"))
	// }

	if w.WorkloadType == nil {
		w.Errors = append(w.Errors, errors.New("workload type is required"))
	} else if (strings.EqualFold(*w.WorkloadType, NexExecutionProviderV8) ||
		strings.EqualFold(*w.WorkloadType, NexExecutionProviderWasm)) &&
		len(w.TriggerSubjects) == 0 {
		w.Errors = append(w.Errors, errors.New("at least one trigger subject is required for this workload type"))
	}

	return len(w.Errors) == 0
}

type DeployResponse struct {
	Accepted bool    `json:"accepted"`
	Message  *string `json:"message"`
}

type HandshakeRequest struct {
	MachineId *string   `json:"machine_id"`
	StartTime time.Time `json:"start_time"`
	Message   *string   `json:"message,omitempty"`
}

type MachineMetadata struct {
	VmId         *string `json:"vmid"`
	NodeNatsHost *string `json:"node_nats_host"`
	NodeNatsPort *int    `json:"node_nats_port"`
	Message      *string `json:"message"`

	Errors []error `json:"errors,omitempty"`
}

func (m *MachineMetadata) Validate() bool {
	m.Errors = make([]error, 0)

	if m.VmId == nil {
		m.Errors = append(m.Errors, errors.New("vm id is required"))
	}

	if m.NodeNatsHost == nil {
		m.Errors = append(m.Errors, errors.New("node NATS host is required"))
	}

	if m.NodeNatsPort == nil {
		m.Errors = append(m.Errors, errors.New("node NATS port is required"))
	}

	return len(m.Errors) == 0
}

type LogEntry struct {
	Source string   `json:"source,omitempty"`
	Level  LogLevel `json:"level,omitempty"`
	Text   string   `json:"text,omitempty"`
}

type LogLevel int32
