package nexnode

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

var (
	// docker/OCI needs to be explicitly enabled in node configuration
	defaultWorkloadTypes = []string{"elf", "v8", "wasm"}
)

// Node configuration is used to configure the node process as well as the virtual machines it
// produces
type NodeConfiguration struct {
	KernelFile       string            `json:"kernel_file"`
	RootFsFile       string            `json:"rootfs_file"`
	DefaultDir       string            `json:"default_resource_dir"`
	MachinePoolSize  int               `json:"machine_pool_size"`
	CNI              CNIDefinition     `json:"cni"`
	MachineTemplate  MachineTemplate   `json:"machine_template"`
	RateLimiters     *Limiters         `json:"rate_limiters,omitempty"`
	ValidIssuers     []string          `json:"valid_issuers,omitempty"`
	InternalNodeHost string            `json:"internal_node_host,omitempty"`
	InternalNodePort int               `json:"internal_node_port"`
	WorkloadTypes    []string          `json:"workload_types,omitempty"`
	Tags             map[string]string `json:"tags,omitempty"`
	ForensicMode     bool              `json:"-"`
	ForceDepInstall  bool              `json:"-"`
}

// A set of rate limiters. These fields are identical to those in firecracker rate limiter configuration
type Limiters struct {
	Bandwidth  *TokenBucket `json:"bandwidth,omitempty"`
	Operations *TokenBucket `json:"iops,omitempty"`
}

// Defines a reference to the CNI network name, which is defined and configured in a {network}.conflist file, as per
// CNI convention
type CNIDefinition struct {
	NetworkName   string `json:"network_name"`
	InterfaceName string `json:"interface_name"`
}

// Defines the CPU and memory usage of a machine to be configured when it is added to the pool
type MachineTemplate struct {
	VcpuCount  int `json:"vcpu_count"`
	MemSizeMib int `json:"memsize_mib"`
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
	return NodeConfiguration{
		MachinePoolSize: 1,
		CNI: CNIDefinition{
			NetworkName:   "fcnet",
			InterfaceName: "veth0",
		},
		MachineTemplate: MachineTemplate{
			VcpuCount:  1,
			MemSizeMib: 256,
		},
		Tags:          make(map[string]string),
		RateLimiters:  nil,
		WorkloadTypes: defaultWorkloadTypes,
		// CAUTION: This needs to be the IP of the node server's internal NATS --as visible to the inside of the firecracker VM--. This is not necessarily the address
		// on which the internal NATS server is actually listening on inside the node.
		InternalNodeHost: "192.168.127.1",
		InternalNodePort: 9222,
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

	if config.KernelFile == "" && config.DefaultDir != "" {
		config.KernelFile = filepath.Join(config.DefaultDir, "vmlinux")
	} else if config.KernelFile == "" && config.DefaultDir == "" {
		return nil, errors.New("invalid kernel file setting")
	}
	if config.RootFsFile == "" && config.DefaultDir != "" {
		config.RootFsFile = filepath.Join(config.DefaultDir, "rootfs.ext4")
	} else if config.KernelFile == "" && config.DefaultDir == "" {
		return nil, errors.New("invalid rootfs file setting")
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
	// When enabled, the nex node will not clean up logs or rootfs files
	ForensicMode bool
	// When enabled, preflight will automatically install or reinstall dependencies
	ForceDependencyInstall bool
}
