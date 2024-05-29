package node

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/cli/globals"
	"github.com/synadia-io/nex/internal/models"
)

type UpCmd struct {
	AgentHandshakeTimeoutMillisecond int                  `default:"5000" group:"Nex Node Up Configuration" json:"up_agent_handshake_timeout_ms"`
	DefaultResourceDir               string               `default:"./resources" group:"Nex Node Up Configuration" json:"up_default_resource_dir"`
	InternalNodeHost                 string               `default:"192.168.127.1" group:"Nex Node Up Configuration" json:"up_internal_node_host"`
	InternalNodePort                 int                  `default:"9222" group:"Nex Node Up Configuration" json:"up_internal_node_port"`
	FirecrackerBinPath               string               `default:"/usr/local/bin/firecracker" group:"Nex Node Up Configuration" json:"up_firecracker_bin"`
	MachinePoolSize                  int                  `default:"1" group:"Nex Node Up Configuration" json:"up_machine_pool_size"`
	NoSandbox                        bool                 `default:"false" group:"Nex Node Up Configuration" json:"up_no_sandbox"`
	PreserveNetwork                  bool                 `default:"true" group:"Nex Node Up Configuration" json:"up_preserve_network"`
	KernelFilepath                   string               `group:"Nex Node Up Configuration" json:"up_kernel_filepath"`
	RootFsFilepath                   string               `group:"Nex Node Up Configuration" json:"up_rootfs_filepath"`
	Tags                             map[string]string    `placeholder:"nex:iscool;..." group:"Nex Node Up Configuration" json:"up_tags"`
	ValidIssuers                     []string             `group:"Nex Node Up Configuration" json:"up_valid_issuers"`
	WorkloadTypes                    []models.NexWorkload `default:"native" enum:"native,function,container,wasm" group:"Nex Node Up Configuration" json:"up_workload_types"`
	FunctionLanguage                 string               `default:"javascript" enum:"javascript" hidden:"" json:"-"`
	NexusName                        string               `default:"nexus" group:"Nex Node Up Configuration" json:"up_nexus_name"`
	PublicNATSServer                 []byte               `placeholder:"./path/to/nats_server.conf" type:"filecontent" help:"Path to nats server config to be used for userland NATS server. JSON format" group:"Nex Node Up Configuration"`

	CNIDefinition      *CNIDefinition      `embed:"" group:"CNI Configuration"`
	MachineTemplate    *MachineTemplate    `embed:"" group:"Firecracker Machine Configuration"`
	Limiters           *Limiters           `embed:"" prefix:"limiter_" group:"Limiter Configuration"`
	HostServicesConfig *HostServicesConfig `embed:"" group:"Host Services Configuration"`
	OtelConfig         *OtelConfig         `embed:"" group:"OpenTelemetry Configuration"`
}

type OtelConfig struct {
	OtelMetrics         bool   `name:"metrics" default:"false" help:"Enables OTel Metrics" json:"up_otel_metrics_enabled"`
	OtelMetricsPort     int    `default:"8085" json:"up_otel_metrics_port"`
	OtelMetricsExporter string `default:"file" enum:"file,prometheus" json:"up_otel_metrics_exporter"`
	OtelTraces          bool   `name:"traces" default:"false" help:"Enables OTel Traces" json:"up_otel_traces_enabled"`
	OtelTracesExporter  string `default:"file" enum:"file,grpc,http" json:"up_otel_traces_exporter"`
	OtlpExporterUrl     string `default:"127.0.0.1:14532" json:"up_otlp_exporter_url"`
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

func (u UpCmd) Run(ctx context.Context, nc *nats.Conn, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), u.Table())
	}
	return nil
}

func (u UpCmd) Validate() error {
	return nil
}
