package agentapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	controlapi "github.com/synadia-io/nex/control-api"
)

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

	// NATS connections which be injected into the execution provider
	NATSConn *nats.Conn `json:"-"`

	PluginPath *string `json:"-"`
}

// DeployRequest processed by the agent
type DeployRequest struct {
	Argv          []string          `json:"argv,omitempty"`
	DecodedClaims jwt.GenericClaims `json:"-"`
	Description   *string           `json:"description"`
	Environment   map[string]string `json:"environment"`
	Essential     *bool             `json:"essential,omitempty"`
	Hash          string            `json:"hash,omitempty"`
	ID            *string           `json:"id"`
	Namespace     *string           `json:"namespace,omitempty"`
	RetriedAt     *time.Time        `json:"retried_at,omitempty"`
	RetryCount    *uint             `json:"retry_count,omitempty"`
	TotalBytes    int64             `json:"total_bytes,omitempty"`

	HostServicesConfig *controlapi.NatsJwtConnectionInfo `json:"host_services_config,omitempty"`
	TriggerSubjects    []string                          `json:"trigger_subjects"`
	WorkloadName       *string                           `json:"workload_name,omitempty"`
	WorkloadType       controlapi.NexWorkload            `json:"workload_type,omitempty"`

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

func (request *DeployRequest) IsEssential() bool {
	return request.Essential != nil && *request.Essential
}

// Returns true if the run request supports essential flag
func (request *DeployRequest) SupportsEssential() bool {
	return request.WorkloadType == controlapi.NexWorkloadNative ||
		request.WorkloadType == controlapi.NexWorkloadOCI
}

// Returns true if the run request supports trigger subjects
func (request *DeployRequest) SupportsTriggerSubjects() bool {
	return (request.WorkloadType == controlapi.NexWorkloadV8 ||
		request.WorkloadType == controlapi.NexWorkloadWasm) &&
		len(request.TriggerSubjects) > 0
}

func (r *DeployRequest) Validate() error {
	var err error

	if r.Namespace == nil {
		err = errors.Join(err, errors.New("namespace is required"))
	}

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

	if r.WorkloadType == "" {
		err = errors.Join(err, errors.New("workload type is required"))
	} else if (r.WorkloadType == controlapi.NexWorkloadV8 ||
		r.WorkloadType == controlapi.NexWorkloadWasm) &&
		len(r.TriggerSubjects) == 0 {
		err = errors.Join(err, errors.New("at least one trigger subject is required for this workload type"))
	}

	return err
}

type DeployResponse struct {
	Accepted bool    `json:"accepted"`
	Message  *string `json:"message"`
}

type HandshakeRequest struct {
	ID        *string   `json:"id"`
	StartTime time.Time `json:"start_time"`
	Message   *string   `json:"message,omitempty"`
}

type HandshakeResponse struct {
}

type HostServicesHTTPRequest struct {
	Method string `json:"method"`
	URL    string `json:"url"`

	Body    *string          `json:"body,omitempty"`
	Headers *json.RawMessage `json:"headers,omitempty"`

	// FIXME-- this is very poorly named currently...
	//these params are parsed as an object and serialized as part of the query string
	Params *json.RawMessage `json:"params,omitempty"`
}

type HostServicesHTTPResponse struct {
	Status  int              `json:"status"`
	Headers *json.RawMessage `json:"headers,omitempty"`
	Body    string           `json:"body"`

	Error *string `json:"error,omitempty"`
}

type HostServicesKeyValueResponse struct {
	Revision int64 `json:"revision,omitempty"`
	Success  *bool `json:"success,omitempty"`

	Errors []string `json:"errors,omitempty"`
}

type HostServicesObjectStoreResponse struct {
	Errors  []string `json:"errors,omitempty"`
	Success bool     `json:"success,omitempty"`
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
	VmID             *string `json:"vmid"`
	NodeNatsHost     *string `json:"node_nats_host"`
	NodeNatsPort     *int    `json:"node_nats_port"`
	NodeNatsNkeySeed *string `json:"node_nats_nkey"`
	Message          *string `json:"message"`
	PluginPath       *string `json:"plugin_path,omitempty"`

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
	Source string     `json:"source,omitempty"`
	Level  slog.Level `json:"level,omitempty"`
	Text   string     `json:"text,omitempty"`
}
