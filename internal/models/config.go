package models

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/splode/fname"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const (
	DefaultCNINetworkName   = "fcnet"
	DefaultCNIInterfaceName = "veth0"
	DefaultCNISubnet        = "192.168.127.0/24"
	DefaultInternalNodeHost = "192.168.127.1"
	DefaultInternalNodePort = 9222
	DefaultNodeMemSizeMib   = 256
	DefaultNodeVcpuCount    = 1
	DefaultOtelExporterUrl  = "127.0.0.1:14532"
)

var (
	// docker/OCI needs to be explicitly enabled in node configuration
	DefaultWorkloadTypes = []string{"elf", "v8", "wasm"}

	DefaultBinPath = append([]string{"/usr/local/bin"}, filepath.SplitList(os.Getenv("PATH"))...)

	// check the default cni bin path first, otherwise look in the rest of the PATH
	DefaultCNIBinPath = append([]string{"/opt/cni/bin"}, filepath.SplitList(os.Getenv("PATH"))...)
)

// Node configuration is used to configure the node process as well
// as the virtual machines it produces
type NodeConfiguration struct {
	AgentHandshakeTimeoutMillisecond int               `json:"agent_handshake_timeout_ms,omitempty"`
	BinPath                          []string          `json:"bin_path"`
	CNI                              CNIDefinition     `json:"cni"`
	DefaultResourceDir               string            `json:"default_resource_dir"`
	ForceDepInstall                  bool              `json:"-"`
	InternalNodeHost                 *string           `json:"internal_node_host,omitempty"`
	InternalNodePort                 *int              `json:"internal_node_port"`
	KernelFilepath                   string            `json:"kernel_filepath"`
	MachinePoolSize                  int               `json:"machine_pool_size"`
	MachineTemplate                  MachineTemplate   `json:"machine_template"`
	NoSandbox                        bool              `json:"no_sandbox,omitempty"`
	OtlpExporterUrl                  string            `json:"otlp_exporter_url,omitempty"`
	OtelMetrics                      bool              `json:"otel_metrics"`
	OtelMetricsPort                  int               `json:"otel_metrics_port"`
	OtelMetricsExporter              string            `json:"otel_metrics_exporter"`
	OtelTraces                       bool              `json:"otel_traces"`
	OtelTracesExporter               string            `json:"otel_traces_exporter"`
	PreserveNetwork                  bool              `json:"preserve_network,omitempty"`
	RateLimiters                     *Limiters         `json:"rate_limiters,omitempty"`
	RootFsFilepath                   string            `json:"rootfs_filepath"`
	Tags                             map[string]string `json:"tags,omitempty"`
	ValidIssuers                     []string          `json:"valid_issuers,omitempty"`
	WorkloadTypes                    []string          `json:"workload_types,omitempty"`

	// Public NATS server options; when non-nil, a public "userland" NATS server is started during node init
	PublicNATSServer *server.Options `json:"public_nats_server,omitempty"`

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

	cniSubnet, err := netip.ParsePrefix(*c.CNI.Subnet)
	if err != nil {
		c.Errors = append(c.Errors, err)
	}
	internalNodeHost, err := netip.ParseAddr(*c.InternalNodeHost)
	if err != nil {
		c.Errors = append(c.Errors, err)
	}

	hostInSubnet := cniSubnet.Contains(internalNodeHost)
	if !hostInSubnet {
		c.Errors = append(c.Errors, errors.New("internal node host must be in the CNI subnet"))
	}

	return len(c.Errors) == 0
}

func DefaultNodeConfiguration() NodeConfiguration {
	defaultNodePort := DefaultInternalNodePort
	defaultVcpuCount := DefaultNodeVcpuCount
	defaultMemSizeMib := DefaultNodeMemSizeMib

	tags := make(map[string]string)
	rng := fname.NewGenerator()
	nodeName, err := rng.Generate()
	if err == nil {
		tags["node_name"] = nodeName
	}

	return NodeConfiguration{
		AgentHandshakeTimeoutMillisecond: 5000,
		BinPath:                          DefaultBinPath,
		CNI: CNIDefinition{
			BinPath:       DefaultCNIBinPath,
			NetworkName:   agentapi.StringOrNil(DefaultCNINetworkName),
			InterfaceName: agentapi.StringOrNil(DefaultCNIInterfaceName),
			Subnet:        agentapi.StringOrNil(DefaultCNISubnet),
		},
		// CAUTION: This needs to be the IP of the node server's internal NATS --as visible to the inside of the firecracker VM--. This is not necessarily the address
		// on which the internal NATS server is actually listening on inside the node.
		InternalNodeHost: agentapi.StringOrNil(DefaultInternalNodeHost),
		InternalNodePort: &defaultNodePort,
		MachinePoolSize:  1,
		MachineTemplate: MachineTemplate{
			VcpuCount:  &defaultVcpuCount,
			MemSizeMib: &defaultMemSizeMib,
		},
		OtlpExporterUrl: DefaultOtelExporterUrl,
		Tags:            tags,
		RateLimiters:    nil,
		WorkloadTypes:   DefaultWorkloadTypes,
	}
}
