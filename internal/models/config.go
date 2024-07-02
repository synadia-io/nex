package models

import (
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"path/filepath"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/splode/fname"
	controlapi "github.com/synadia-io/nex/control-api"
)

const (
	DefaultCNINetworkName                   = "fcnet"
	DefaultCNIInterfaceName                 = "veth0"
	DefaultCNISubnet                        = "192.168.127.0/24"
	DefaultInternalNodeHost                 = "192.168.127.1"
	DefaultInternalNodePort                 = 9222
	DefaultNodeMemSizeMib                   = 256
	DefaultNodeVcpuCount                    = 1
	DefaultOtelExporterUrl                  = "127.0.0.1:14532"
	DefaultAgentHandshakeTimeoutMillisecond = 5000
	DefaultAgentPingTimeoutMillisecond      = 750
)

var (
	RequiredTriggerSubjectDenyList = []string{"$SYS.>", "$JS.>", "$NEX.>"}
	DefaultWorkloadTypes           = []controlapi.NexWorkload{controlapi.NexWorkloadNative}

	DefaultBinPath    = buildDefaultBinPath()
	DefaultCNIBinPath = filepath.SplitList(os.Getenv("PATH"))
)

// Node configuration is used to configure the node process as well
// as the virtual machines it produces
type NodeConfiguration struct {
	AgentHandshakeTimeoutMillisecond int                      `json:"agent_handshake_timeout_ms,omitempty"`
	AgentPingTimeoutMillisecond      int                      `json:"agent_ping_timeout_ms,omitempty"`
	AutostartConfiguration           *AutostartConfig         `json:"autostart,omitempty"`
	BinPath                          []string                 `json:"bin_path"`
	CNI                              CNIDefinition            `json:"cni"`
	DefaultResourceDir               string                   `json:"default_resource_dir"`
	ForceDepInstall                  bool                     `json:"-"`
	HostServicesConfiguration        *HostServicesConfig      `json:"host_services,omitempty"`
	InternalNodeDebug                bool                     `json:"internal_node_debug,omitempty"`
	InternalNodeHost                 *string                  `json:"internal_node_host,omitempty"`
	InternalNodePort                 *int                     `json:"internal_node_port"`
	InternalNodeStoreDir             *string                  `json:"internal_node_store_dir,omitempty"`
	InternalNodeTrace                bool                     `json:"internal_node_trace,omitempty"`
	KernelFilepath                   string                   `json:"kernel_filepath"`
	MachinePoolSize                  int                      `json:"machine_pool_size"`
	MachineTemplate                  MachineTemplate          `json:"machine_template"`
	NoSandbox                        bool                     `json:"no_sandbox,omitempty"`
	OtlpExporterUrl                  string                   `json:"otlp_exporter_url,omitempty"`
	OtelMetrics                      bool                     `json:"otel_metrics"`
	OtelMetricsPort                  int                      `json:"otel_metrics_port"`
	OtelMetricsExporter              string                   `json:"otel_metrics_exporter"`
	OtelTraces                       bool                     `json:"otel_traces"`
	OtelTracesExporter               string                   `json:"otel_traces_exporter"`
	PidFilepath                      *string                  `json:"pid_filepath"`
	PreserveNetwork                  bool                     `json:"preserve_network,omitempty"`
	RateLimiters                     *Limiters                `json:"rate_limiters,omitempty"`
	RootFsFilepath                   string                   `json:"rootfs_filepath"`
	Tags                             map[string]string        `json:"tags,omitempty"`
	ValidIssuers                     []string                 `json:"valid_issuers,omitempty"`
	WorkloadTypes                    []controlapi.NexWorkload `json:"workload_types,omitempty"`
	AgentPluginPath                  *string                  `json:"agent_plugin_path,omitempty"`
	DenyTriggerSubjects              []string                 `json:"deny_trigger_subjects,omitempty"`
	NodeLimits                       NodeLimitsConfig         `json:"node_limits"`

	PreflightVerbose        bool   `json:"-"`
	PreflightVerify         bool   `json:"-"`
	PreflightCheck          bool   `json:"-"`
	PreflightInstallVersion string `json:"-"`
	PreflightYes            bool   `json:"-"`

	// Public NATS server options; when non-nil, a public "userland" NATS server is started during node init
	PublicNATSServer *server.Options `json:"public_nats_server,omitempty"`

	Errors []error `json:"errors,omitempty"`
}

// FIXME-- these properties should probably be *string 👀
type HostServicesConfig struct {
	NatsUrl      string                   `json:"nats_url"`
	NatsUserJwt  string                   `json:"nats_user_jwt"`
	NatsUserSeed string                   `json:"nats_user_seed"`
	Services     map[string]ServiceConfig `json:"services"`
}

type NodeLimitsConfig struct {
	MaxWorkloads     int `json:"max_workloads"`
	MaxTotalBytes    int `json:"max_total_bytes"`
	MaxWorkloadBytes int `json:"max_workload_bytes"`
}

type ServiceConfig struct {
	Enabled       bool            `json:"enabled"`
	Configuration json.RawMessage `json:"config"`
}

type AutostartConfig struct {
	Workloads []AutostartDeployRequest `json:"workloads"`
}

type AutostartDeployRequest struct {
	Name            string                 `json:"name"`
	Namespace       string                 `json:"namespace"`
	Argv            []string               `json:"argv,omitempty"`
	Description     *string                `json:"description,omitempty"`
	Essential       bool                   `json:"essential,omitempty"`
	WorkloadType    controlapi.NexWorkload `json:"type"`
	Location        string                 `json:"location"`
	JsDomain        *string                `json:"jsdomain,omitempty"`
	Environment     map[string]string      `json:"environment"`
	TriggerSubjects []string               `json:"trigger_subjects,omitempty"`
}

func (c *NodeConfiguration) Validate() bool {
	c.Errors = make([]error, 0)

	if c.MachinePoolSize < 1 {
		c.Errors = append(c.Errors, errors.New("machine pool size must be >= 1"))
	}

	if !c.NoSandbox {
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

	config := NodeConfiguration{
		AgentHandshakeTimeoutMillisecond: DefaultAgentHandshakeTimeoutMillisecond,
		AgentPingTimeoutMillisecond:      DefaultAgentPingTimeoutMillisecond,
		BinPath:                          DefaultBinPath,
		// CAUTION: This needs to be the IP of the node server's internal NATS --as visible to the agent.
		// This is not necessarily the address on which the internal NATS server is actually listening inside the node.
		InternalNodeHost: StringOrNil(DefaultInternalNodeHost),
		InternalNodePort: &defaultNodePort,
		MachinePoolSize:  1,
		MachineTemplate: MachineTemplate{
			VcpuCount:  &defaultVcpuCount,
			MemSizeMib: &defaultMemSizeMib,
		},
		OtlpExporterUrl: DefaultOtelExporterUrl,
		RateLimiters:    nil,
		Tags:            tags,
		WorkloadTypes:   DefaultWorkloadTypes,
		HostServicesConfiguration: &HostServicesConfig{
			NatsUrl:      "", // this will trigger logic to re-use the main connection
			NatsUserJwt:  "",
			NatsUserSeed: "",
			Services: map[string]ServiceConfig{
				"http": {
					Enabled:       true,
					Configuration: nil,
				},
				"kv": {
					Enabled:       true,
					Configuration: nil,
				},
				"messaging": {
					Enabled:       true,
					Configuration: nil,
				},
				"objectstore": {
					Enabled:       true,
					Configuration: nil,
				},
			},
		},
	}

	if !config.NoSandbox {
		config.CNI = CNIDefinition{
			BinPath:       DefaultCNIBinPath,
			NetworkName:   StringOrNil(DefaultCNINetworkName),
			InterfaceName: StringOrNil(DefaultCNIInterfaceName),
			Subnet:        StringOrNil(DefaultCNISubnet),
		}
	}

	return config
}

func buildDefaultBinPath() []string {
	homedir, err := os.UserHomeDir()
	if err != nil {
		return filepath.SplitList(os.Getenv("PATH"))
	}

	defaultBinPath := filepath.Join(homedir, ".nex", "bin")
	_ = os.MkdirAll(defaultBinPath, 0755)

	path := make([]string, 0)
	path = append(path, defaultBinPath)
	path = append(path, filepath.SplitList(os.Getenv("PATH"))...)

	return path
}
