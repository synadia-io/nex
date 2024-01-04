package nexnode

import (
	"encoding/json"
	"os"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

const defaultCNINetworkName = "fcnet"
const defaultCNIInterfaceName = "veth0"
const defaultInternalNodeHost = "192.168.127.1"
const defaultInternalNodePort = 9222
const defaultKernelPath = "vmlinux-5.10"
const defaultNodeMemSizeMib = 256
const defaultNodeVcpuCount = 1
const defaultRootFsPath = "rootfs.ext4"

var (
	// docker/OCI needs to be explicitly enabled in node configuration
	defaultWorkloadTypes = []string{"elf", "v8", "wasm"}
)

// Node configuration is used to configure the node process as well
// as the virtual machines it produces
type NodeConfiguration struct {
	CNI              CNIDefinition     `json:"cni"`
	InternalNodeHost *string           `json:"internal_node_host,omitempty"`
	InternalNodePort *int              `json:"internal_node_port"`
	KernelPath       *string           `json:"kernel_path"`
	MachinePoolSize  int               `json:"machine_pool_size"`
	MachineTemplate  MachineTemplate   `json:"machine_template"`
	RateLimiters     *Limiters         `json:"rate_limiters,omitempty"`
	RootFsPath       *string           `json:"rootfs_path"`
	Tags             map[string]string `json:"tags,omitempty"`
	ValidIssuers     []string          `json:"valid_issuers,omitempty"`
	WorkloadTypes    []string          `json:"workload_types,omitempty"`

	Errors []error `json:"errors,omitempty"`
}

func (c *NodeConfiguration) Validate() bool {
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

func DefaultNodeConfiguration() NodeConfiguration {
	defaultNodePort := defaultInternalNodePort
	defaultVcpuCount := defaultNodeVcpuCount
	defaultMemSizeMib := defaultNodeMemSizeMib

	return NodeConfiguration{
		CNI: CNIDefinition{
			NetworkName:   agentapi.StringOrNil(defaultCNINetworkName),
			InterfaceName: agentapi.StringOrNil(defaultCNIInterfaceName),
		},
		// CAUTION: This needs to be the IP of the node server's internal NATS --as visible to the inside of the firecracker VM--. This is not necessarily the address
		// on which the internal NATS server is actually listening on inside the node.
		InternalNodeHost: agentapi.StringOrNil(defaultInternalNodeHost),
		InternalNodePort: &defaultNodePort,
		KernelPath:       agentapi.StringOrNil(defaultKernelPath),
		MachinePoolSize:  1,
		MachineTemplate: MachineTemplate{
			VcpuCount:  &defaultVcpuCount,
			MemSizeMib: &defaultMemSizeMib,
		},
		Tags:          make(map[string]string),
		RateLimiters:  nil,
		RootFsPath:    agentapi.StringOrNil(defaultRootFsPath),
		WorkloadTypes: defaultWorkloadTypes,
	}
}

// Retrieves the node configuration from the specified file
func LoadNodeConfiguration(configFilePath string) (*NodeConfiguration, error) {
	bytes, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, err
	}
	config := DefaultNodeConfiguration()
	err = json.Unmarshal(bytes, &config)
	if len(config.WorkloadTypes) == 0 {
		config.WorkloadTypes = defaultWorkloadTypes
	}
	if err != nil {
		return nil, err
	}
	if config.Tags == nil {
		config.Tags = make(map[string]string)
	}
	return &config, nil
}

type CliOptions struct {
	// Servers is the list of servers to connect to
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
	// TlsFirst configures theTLSHandshakeFirst behavior in nats.go
	TlsFirst bool
	// Path to the file containing virtual machine management settings and node-wide settings
	NodeConfigFile string
}
