package agentapi

import (
	"errors"
	"io"
	"strings"
	"time"

	"github.com/nats-io/jwt/v2"
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

// DeployRequest processed by the agent
type DeployRequest struct {
	Argv            []string          `json:"argv,omitempty"`
	DecodedClaims   jwt.GenericClaims `json:"-"`
	Description     *string           `json:"description"`
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

// Returns true if the run request supports trigger subjects
func (request *DeployRequest) SupportsTriggerSubjects() bool {
	return (strings.EqualFold(*request.WorkloadType, "v8") ||
		strings.EqualFold(*request.WorkloadType, "wasm")) &&
		len(request.TriggerSubjects) > 0
}

func (r *DeployRequest) Validate() bool {
	var err error

	if r.WorkloadName == nil {
		err = errors.Join(errors.New("workload name is required"))
	}

	if r.Hash == "" { // FIXME--- this should probably be checked against *string
		err = errors.Join(errors.New("hash is required"))
	}

	if r.TotalBytes == 0 { // FIXME--- this should probably be checked against *string
		err = errors.Join(errors.New("total bytes is required"))
	}

	if r.WorkloadType == nil {
		err = errors.Join(errors.New("workload type is required"))
	} else if (strings.EqualFold(*r.WorkloadType, NexExecutionProviderV8) ||
		strings.EqualFold(*r.WorkloadType, NexExecutionProviderWasm)) &&
		len(r.TriggerSubjects) == 0 {
		err = errors.Join(errors.New("at least one trigger subject is required for this workload type"))
	}

	return err == nil
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
	var err error

	if m.VmId == nil {
		err = errors.Join(errors.New("vm id is required"))
	}

	if m.NodeNatsHost == nil {
		err = errors.Join(errors.New("node NATS host is required"))
	}

	if m.NodeNatsPort == nil {
		err = errors.Join(errors.New("node NATS port is required"))
	}

	return err == nil
}

type LogEntry struct {
	Source string   `json:"source,omitempty"`
	Level  LogLevel `json:"level,omitempty"`
	Text   string   `json:"text,omitempty"`
}

type LogLevel int32
