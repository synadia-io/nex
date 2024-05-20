package up

import (
	"encoding/json"
	"os"
)

type UpCmd struct {
	AgentHandshakeTimeoutMillisecond int               `default:"5000" group:"Nex Node Configuration" json:"up_agent_handshake_timeout_ms"`
	DefaultResourceDir               string            `default:"./resources" group:"Nex Node Configuration" json:"up_default_resource_dir"`
	InternalNodeHost                 string            `default:"192.168.127.1" group:"Nex Node Configuration" json:"up_internal_node_host"`
	InternalNodePort                 int               `default:"9222" group:"Nex Node Configuration" json:"up_internal_node_port"`
	MachinePoolSize                  int               `default:"1" group:"Nex Node Configuration" json:"up_machine_pool_size"`
	NoSandbox                        bool              `default:"false" group:"Nex Node Configuration" json:"up_no_sandbox"`
	PreserveNetwork                  bool              `default:"true" group:"Nex Node Configuration" json:"up_preserve_network"`
	KernelFilepath                   *os.File          `group:"Nex Node Configuration" json:"up_kernel_filepath"`
	RootFsFilepath                   *os.File          `group:"Nex Node Configuration" json:"up_rootfs_filepath"`
	Tags                             map[string]string `placeholder:"nex:iscool;..." group:"Nex Node Configuration" json:"up_tags"`
	ValidIssuers                     []string          `group:"Nex Node Configuration" json:"up_valid_issuers"`
	WorkloadType                     string            `default:"daemon" enum:"daemon,function,container,wasm" group:"Nex Node Configuration" json:"up_workload_type"`
	FunctionLanguage                 string            `default:"javascript" enum:"javascript" hidden:"" json:"-"`
	CNIDefinition                    `group:"CNI Configuration"`
	MachineTemplate                  `group:"Firecracker Machine Configuration"`
	Limiters                         `embed:"" prefix:"limiter_" group:"Limiter Configuration"`
	HostServicesConfig               `group:"Host Services Configuration"`
	PublicNATSServer                 []byte `placeholder:"./path/to/nats_server.conf" type:"filecontent" help:"Path to nats server config to be used for userland NATS server. JSON format" group:"Nex Node Configuration"`

	OtelMetrics         bool   `name:"metrics" default:"false" help:"Enables OTel Metrics" group:"OpenTelemetry Configuration" json:"up_otel_metrics" json:"up_otel_metrics_enabled"`
	OtelMetricsPort     int    `default:"8085" group:"OpenTelemetry Configuration" json:"up_otel_metrics_port"`
	OtelMetricsExporter string `default:"file" enum:"file,prometheus" group:"OpenTelemetry Configuration" json:"up_otel_metrics_exporter"`
	OtelTraces          bool   `name:"traces" default:"false" help:"Enables OTel Traces" group:"OpenTelemetry Configuration" json:"up_otel_traces_enabled"`
	OtelTracesExporter  string `default:"file" enum:"file,grpc,http" group:"OpenTelemetry Configuration" json:"up_otel_traces_exporter"`
	OtlpExporterUrl     string `default:"127.0.0.1:14532" group:"OpenTelemetry Configuration" json:"up_otlp_exporter_url"`

	Errors []error `kong:"-"`
}

type HostServicesConfig struct {
	NatsUrl      string                   `group:"Host Services Configuration" json:"hostservices_nats_url"`
	NatsUserJwt  string                   `group:"Host Services Configuration" json:"hostservices_nats_user_jwt"`
	NatsUserSeed string                   `group:"Host Services Configuration" json:"hostservices_nats_user_seed"`
	Services     map[string]ServiceConfig `hidden:"" group:"Host Services Configuration" json:"hostservices_services_map"`
}

type ServiceConfig struct {
	Enabled       bool            `group:"Services Configuration" json:"service_enabled"`
	Configuration json.RawMessage `group:"Services Configuration" json:"service_config"`
}

type Limiters struct {
	Bandwidth  *TokenBucket `embed:"" prefix:"bandwidth_" json:"limiters_bandwidth"`
	Operations *TokenBucket `embed:"" prefix:"operations_" json:"limiters_operations"`
}

// Defines a reference to the CNI network name, which is defined and configured in a {network}.conflist file, as per
// CNI convention
type CNIDefinition struct {
	CniBinPaths      []string `type:"existingdir" default:"/opt/cni/bin" json:"cni_bin_paths"`
	CniInterfaceName string   `default:"veth0" json:"cni_interface_name"`
	CniNetworkName   string   `default:"fcnet" json:"cni_network_name"`
	CniSubnet        string   `default:"192.168.127.0/24" json:"cni_subnet"`
}

// Defines the CPU and memory usage of a machine to be configured when it is added to the pool
type MachineTemplate struct {
	FirecrackerVcpuCount  int `default:"1" json:"machine_template_firecracker_cpu_count"`
	FirecrackerMemSizeMib int `default:"256" json:"machine_template_firecracker_mem_size_mib"`
}

type TokenBucket struct {
	// The initial size of a token bucket.
	// Minimum: 0
	OneTimeBurst int64 `placeholder:"0" json:"token_bucket_one_time_burst"`

	// The amount of milliseconds it takes for the bucket to refill.
	// Required: true
	// Minimum: 0
	RefillTime int64 `placeholder:"0" json:"token_bucket_refill_time"`

	// The total number of tokens this bucket can hold.
	// Required: true
	// Minimum: 0
	Size int64 `placeholder:"0" json:"token_bucket_size"`
}
