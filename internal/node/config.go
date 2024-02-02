package nexnode

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const defaultCNINetworkName = "fcnet"
const defaultCNIInterfaceName = "veth0"
const defaultInternalNodeHost = "192.168.127.1"
const defaultInternalNodePort = 9222
const defaultNodeMemSizeMib = 256
const defaultNodeVcpuCount = 1

var (
	// docker/OCI needs to be explicitly enabled in node configuration
	defaultWorkloadTypes = []string{"elf", "v8", "wasm"}
	defaultBinPath       = append([]string{"/usr/local/bin"}, filepath.SplitList(os.Getenv("PATH"))...)
	// check the default cni bin path first, otherwise look in the rest of the PATH
	defaultCNIBinPath = append([]string{"/opt/cni/bin"}, filepath.SplitList(os.Getenv("PATH"))...)
)

// Node configuration is used to configure the node process as well
// as the virtual machines it produces
type NodeConfiguration struct {
	BinPath            []string          `json:"bin_path"`
	CNI                 CNIDefinition     `json:"cni"`
	DefaultResourceDir  string            `json:"default_resource_dir"`
	ForceDepInstall     bool              `json:"-"`
	InternalNodeHost    *string           `json:"internal_node_host,omitempty"`
	InternalNodePort    *int              `json:"internal_node_port"`
	KernelFilepath      string            `json:"kernel_filepath"`
	MachinePoolSize     int               `json:"machine_pool_size"`
	MachineTemplate     MachineTemplate   `json:"machine_template"`
	OtelMetrics         bool              `json:"otel_metrics"`
	OtelMetricsPort     int               `json:"otel_metrics_port"`
	OtelMetricsExporter string            `json:"otel_metrics_exporter"`
	PreserveNetwork     bool              `json:"preserve_network,omitempty"`
	RateLimiters        *Limiters         `json:"rate_limiters,omitempty"`
	RootFsFilepath      string            `json:"rootfs_filepath"`
	Tags                map[string]string `json:"tags,omitempty"`
	ValidIssuers        []string          `json:"valid_issuers,omitempty"`
	WorkloadTypes       []string          `json:"workload_types,omitempty"`

	Errors []error `json:"errors,omitempty"`
}

func (c *NodeConfiguration) Validate() bool {
	c.Errors = make([]error, 0)

	if c.MachinePoolSize < 1 {
		c.Errors = append(c.Errors, errors.New("machine pool size must be >= 1"))
	}

	if _, err := os.Stat(c.KernelFilepath); errors.Is(err, os.ErrNotExist) {
		c.Errors = append(c.Errors, err)
	}

	if _, err := os.Stat(c.RootFsFilepath); errors.Is(err, os.ErrNotExist) {
		c.Errors = append(c.Errors, err)
	}

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
	BinPath       []string `json:"bin_path"`
	InterfaceName *string  `json:"interface_name"`
	NetworkName   *string  `json:"network_name"`
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
		BinPath: defaultBinPath,
		CNI: CNIDefinition{
			BinPath:       defaultCNIBinPath,
			NetworkName:   agentapi.StringOrNil(defaultCNINetworkName),
			InterfaceName: agentapi.StringOrNil(defaultCNIInterfaceName),
		},
		// CAUTION: This needs to be the IP of the node server's internal NATS --as visible to the inside of the firecracker VM--. This is not necessarily the address
		// on which the internal NATS server is actually listening on inside the node.
		InternalNodeHost: agentapi.StringOrNil(defaultInternalNodeHost),
		InternalNodePort: &defaultNodePort,
		MachinePoolSize:  1,
		MachineTemplate: MachineTemplate{
			VcpuCount:  &defaultVcpuCount,
			MemSizeMib: &defaultMemSizeMib,
		},
		Tags:          make(map[string]string),
		RateLimiters:  nil,
		WorkloadTypes: defaultWorkloadTypes,
	}
}

// Reads the node configuration from the specified configuration file path
func LoadNodeConfiguration(configFilepath string) (*NodeConfiguration, error) {
	bytes, err := os.ReadFile(configFilepath)
	if err != nil {
		return nil, err
	}

	config := DefaultNodeConfiguration()
	err = json.Unmarshal(bytes, &config)
	if err != nil {
		return nil, err
	}

	if len(config.WorkloadTypes) == 0 {
		config.WorkloadTypes = defaultWorkloadTypes
	}

	// TODO-- audit for *string
	if config.KernelFilepath == "" && config.DefaultResourceDir != "" {
		config.KernelFilepath = filepath.Join(config.DefaultResourceDir, "vmlinux")
	} else if config.KernelFilepath == "" && config.DefaultResourceDir == "" {
		return nil, errors.New("invalid kernel file setting")
	}

	// TODO-- audit for *string
	if config.RootFsFilepath == "" && config.DefaultResourceDir != "" {
		config.RootFsFilepath = filepath.Join(config.DefaultResourceDir, "rootfs.ext4")
	} else if config.RootFsFilepath == "" && config.DefaultResourceDir == "" {
		return nil, errors.New("invalid rootfs file setting")
	}

	if config.Tags == nil {
		config.Tags = make(map[string]string)
	}

	return &config, nil
}
