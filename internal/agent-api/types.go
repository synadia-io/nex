package agentapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
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
	Essential       *bool             `json:"essential,omitempty"`
	Hash            string            `json:"hash,omitempty"`
	Namespace       *string           `json:"namespace,omitempty"`
	RetriedAt       *time.Time        `json:"retried_at,omitempty"`
	RetryCount      *uint             `json:"retry_count,omitempty"`
	TotalBytes      int64             `json:"total_bytes,omitempty"`
	TriggerSubjects []string          `json:"trigger_subjects"`
	WorkloadName    *string           `json:"workload_name,omitempty"`
	WorkloadType    *string           `json:"workload_type,omitempty"`

	Stderr      io.Writer `json:"-"`
	Stdout      io.Writer `json:"-"`
	TmpFilename *string   `json:"-"`

	EncryptedEnvironment *string  `json:"-"`
	JsDomain             *string  `json:"-"`
	Location             *url.URL `json:"-"`
	SenderPublicKey      *string  `json:"-"`
	TargetNode           *string  `json:"-"`
	WorkloadJwt          *string  `json:"-"`

	Errors []error `json:"errors,omitempty"`
}

// Returns an array of DNS names to associate with the running workload
func (request *DeployRequest) DNSNames() []string {
	dnsNames := make([]string, 0)

	var dnsName strings.Builder
	if request.Namespace != nil && *request.Namespace != "default" {
		_, _ = dnsName.WriteString(fmt.Sprintf("%s.", *request.Namespace))
	}
	_, _ = dnsName.WriteString(*request.WorkloadName)
	dnsNames = append(dnsNames, dnsName.String()) // FIXME-- read this from the deploy request instead...

	return dnsNames
}

// Returns true if the run request supports essential flag
func (request *DeployRequest) SupportsEssential() bool {
	return strings.EqualFold(*request.WorkloadType, "elf") ||
		strings.EqualFold(*request.WorkloadType, "oci")
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
		err = errors.Join(err, errors.New("workload name is required"))
	}

	if r.Essential != nil && *r.Essential && !r.SupportsEssential() {
		err = errors.Join(err, errors.New("essential flag is not supported for workload type"))
	}

	if r.Hash == "" { // FIXME--- this should probably be checked against *string
		err = errors.Join(err, errors.New("hash is required"))
	}

	if r.TotalBytes == 0 { // FIXME--- this should probably be checked against *string
		err = errors.Join(err, errors.New("total bytes is required"))
	}

	if r.WorkloadType == nil {
		err = errors.Join(err, errors.New("workload type is required"))
	} else if (strings.EqualFold(*r.WorkloadType, NexExecutionProviderV8) ||
		strings.EqualFold(*r.WorkloadType, NexExecutionProviderWasm)) &&
		len(r.TriggerSubjects) == 0 {
		err = errors.Join(err, errors.New("at least one trigger subject is required for this workload type"))
	}

	return err == nil
}

type DeployResponse struct {
	Accepted bool    `json:"accepted"`
	Message  *string `json:"message"`
}

type HandshakeRequest struct {
	MachineID *string   `json:"machine_id"`
	StartTime time.Time `json:"start_time"`
	Message   *string   `json:"message,omitempty"`
}

type HandshakeResponse struct {
}

type HostServicesKeyValueRequest struct {
	Key   *string          `json:"key"`
	Value *json.RawMessage `json:"value,omitempty"`

	Revision int64 `json:"revision,omitempty"`
	Success  *bool `json:"success,omitempty"`
}

type HostServicesMessagingRequest struct {
	Subject *string          `json:"key"`
	Payload *json.RawMessage `json:"payload,omitempty"`
}

type HostServicesMessagingResponse struct {
	Errors  []string `json:"errors,omitempty"`
	Success bool     `json:"success,omitempty"`
}

type MachineMetadata struct {
	Gateway      *string `json:"gateway"`
	Message      *string `json:"message"`
	Nameserver   *string `json:"nameserver"`
	NodeNatsHost *string `json:"node_nats_host"`
	NodeNatsPort *int    `json:"node_nats_port"`
	VmID         *string `json:"vmid"`

	Errors []error `json:"errors,omitempty"`
}

func (m *MachineMetadata) Validate() bool {
	var err error

	if m.VmID == nil {
		err = errors.Join(err, errors.New("vm id is required"))
	}

	if m.NodeNatsHost == nil {
		err = errors.Join(err, errors.New("node NATS host is required"))
	}

	if m.NodeNatsPort == nil {
		err = errors.Join(err, errors.New("node NATS port is required"))
	}

	return err == nil
}

type LogEntry struct {
	Source string   `json:"source,omitempty"`
	Level  LogLevel `json:"level,omitempty"`
	Text   string   `json:"text,omitempty"`
}

type LogLevel int32
