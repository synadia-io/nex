package models

import (
	"net/url"
	"time"
)

type UiOptions struct {
	Port int
}

type DevRunOptions struct {
	Filename string
	// Stop a workload with the same name on a target
	AutoStop bool
}

// Options configure the CLI
type Options struct {
	Servers string
	// Creds is nats credentials to authenticate with
	Creds string
	// TlsCert is the TLS Public Certificate
	TlsCert string
	// TlsKey is the TLS Private Key
	TlsKey string
	// TlsCA is the certificate authority to verify the connection with
	TlsCA string
	// Timeout is how long to wait for operations
	Timeout time.Duration
	// ConnectionName is the name to use for the underlying NATS connection
	ConnectionName string
	// Username is the username or token to connect with
	Username string
	// Password is the password to connect with
	Password string
	// Nkey is the file holding a nkey to connect with
	Nkey string
	// Trace enables verbose debug logging
	Trace bool
	// SocksProxy is a SOCKS5 proxy to use for NATS connections
	SocksProxy string
	// TlsFirst configures theTLSHandshakeFirst behavior in nats.go
	TlsFirst bool
	// Namespace for scoping workload requests
	Namespace string
	// LogLevel is the log level to use
	LogLevel string
	// LogJSON enables JSON logging
	LogJSON bool
}

type RunOptions struct {
	TargetNode        string
	WorkloadUrl       *url.URL
	Name              string
	WorkloadType      string
	Description       string
	PublisherXkeyFile string
	ClaimsIssuerFile  string
	Env               map[string]string
	DevMode           bool
	TriggerSubjects   []string
}

type StopOptions struct {
	TargetNode       string
	WorkloadName     string
	WorkloadId       string
	ClaimsIssuerFile string
}

type WatchOptions struct {
	NodeId       string
	WorkloadId   string
	WorkloadName string
	LogLevel     string
}

// Node configuration is used to configure the node process as well
// as the virtual machines it produces
type NodeOptions struct {
	ConfigFilepath  string `json:"-"`
	ForceDepInstall bool   `json:"-"`

	CNI                CNIDefinition     `json:"cni"`
	DefaultResourceDir string            `json:"default_resource_dir"`
	InternalNodeHost   *string           `json:"internal_node_host,omitempty"`
	InternalNodePort   *int              `json:"internal_node_port"`
	KernelFilepath     string            `json:"kernel_file"`
	KernelPath         *string           `json:"kernel_path"` // FIXME-- audit json
	MachinePoolSize    int               `json:"machine_pool_size"`
	MachineTemplate    MachineTemplate   `json:"machine_template"`
	RateLimiters       *Limiters         `json:"rate_limiters,omitempty"`
	RootFsFilepath     string            `json:"rootfs_file"` // FIXME-- audit json
	RootFsPath         *string           `json:"rootfs_path"`
	Tags               map[string]string `json:"tags,omitempty"`
	ValidIssuers       []string          `json:"valid_issuers,omitempty"`
	WorkloadTypes      []string          `json:"workload_types,omitempty"`

	Errors []error `json:"errors,omitempty"`
}

func (c *NodeOptions) Validate() bool {
	c.Errors = make([]error, 0)

	// TODO-- add validation

	return len(c.Errors) == 0
}

// A set of rate limiters. These fields are identical to those in firecracker rate limiter configuration
type Limiters struct {
	Bandwidth  *TokenBucket `json:"bandwidth,omitempty"`
	Operations *TokenBucket `json:"iops,omitempty"`
}

// Defines a reference to the CNI network name, which is defined and configured in a {network}.conflist file, as per
// CNI convention
type CNIDefinition struct {
	NetworkName   *string `json:"network_name"`
	InterfaceName *string `json:"interface_name"`
}

// Defines the CPU and memory usage of a machine to be configured when it is added to the pool
type MachineTemplate struct {
	VcpuCount  *int `json:"vcpu_count"`
	MemSizeMib *int `json:"memsize_mib"`
}

type TokenBucket struct {
	// The initial size of a token bucket.
	// Minimum: 0
	OneTimeBurst *int64 `json:"one_time_burst,omitempty"`

	// The amount of milliseconds it takes for the bucket to refill.
	// Required: true
	// Minimum: 0
	RefillTime *int64 `json:"refill_time"`

	// The total number of tokens this bucket can hold.
	// Required: true
	// Minimum: 0
	Size *int64 `json:"size"`
}
