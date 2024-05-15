//go:build linux || windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

type NodeExtendedCmds struct {
	Preflight PreflightCmd `cmd:"" json:"-"`
	Up        UpCmd        `cmd:"" json:"-"`
}

func (n NodeExtendedCmds) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("Node Configuration")
	tw.AppendHeader(table.Row{"Command", "Field", "Value", "Type"})
	tw.AppendRows([]table.Row{
		{"up", "Agent Handshake Timeout Millisecond", n.Up.AgentHandshakeTimeoutMillisecond, reflect.TypeOf(n.Up.AgentHandshakeTimeoutMillisecond).String()},
		{"up", "Default Resource Directory", n.Up.DefaultResourceDir, reflect.TypeOf(n.Up.DefaultResourceDir).String()},
		{"up", "Internal Node NATS Host", n.Up.InternalNodeHost, reflect.TypeOf(n.Up.InternalNodeHost).String()},
		{"up", "Internal Node NATS Port", n.Up.InternalNodePort, reflect.TypeOf(n.Up.InternalNodePort).String()},
		{"up", "Machine Pool Size", n.Up.MachinePoolSize, reflect.TypeOf(n.Up.MachinePoolSize).String()},
		{"up", "No Sandbox", n.Up.NoSandbox, reflect.TypeOf(n.Up.NoSandbox).String()},
		{"up", "Preserve Network", n.Up.PreserveNetwork, reflect.TypeOf(n.Up.PreserveNetwork).String()},
		{"up", "Kernel Filepath", n.Up.KernelFilepath, reflect.TypeOf(n.Up.KernelFilepath).String()},
		{"up", "RootFs Filepath", n.Up.RootFsFilepath, reflect.TypeOf(n.Up.RootFsFilepath).String()},
		{"up", "Tags", n.Up.Tags, reflect.TypeOf(n.Up.Tags).String()},
		{"up", "Valid Issuers", n.Up.ValidIssuers, reflect.TypeOf(n.Up.ValidIssuers).String()},
		{"up", "Workload Type", n.Up.WorkloadType, reflect.TypeOf(n.Up.WorkloadType).String()},
		{"up", "CNI Bin Paths", n.Up.CniBinPaths, reflect.TypeOf(n.Up.CniBinPaths).String()},
		{"up", "CNI Interface Name", n.Up.CniInterfaceName, reflect.TypeOf(n.Up.CniInterfaceName).String()},
		{"up", "CNI Network Name", n.Up.CniNetworkName, reflect.TypeOf(n.Up.CniNetworkName).String()},
		{"up", "CNI Subnet", n.Up.CniSubnet, reflect.TypeOf(n.Up.CniSubnet).String()},
		{"up", "Firecracker Vcpu Count", n.Up.FirecrackerVcpuCount, reflect.TypeOf(n.Up.FirecrackerVcpuCount).String()},
		{"up", "Firecracker Mem Size Mib", n.Up.FirecrackerMemSizeMib, reflect.TypeOf(n.Up.FirecrackerMemSizeMib).String()},
		{"up", "Bandwidth One Time Burst", n.Up.Limiters.Bandwidth.OneTimeBurst, reflect.TypeOf(n.Up.Limiters.Bandwidth.OneTimeBurst).String()},
		{"up", "Bandwidth Refill Time", n.Up.Limiters.Bandwidth.RefillTime, reflect.TypeOf(n.Up.Limiters.Bandwidth.RefillTime).String()},
		{"up", "Bandwidth Size", n.Up.Limiters.Bandwidth.Size, reflect.TypeOf(n.Up.Limiters.Bandwidth.Size).String()},
		{"up", "Operations One Time Burst", n.Up.Limiters.Operations.OneTimeBurst, reflect.TypeOf(n.Up.Limiters.Operations.OneTimeBurst).String()},
		{"up", "Operations Refill Time", n.Up.Limiters.Operations.RefillTime, reflect.TypeOf(n.Up.Limiters.Operations.RefillTime).String()},
		{"up", "Operations Size", n.Up.Limiters.Operations.Size, reflect.TypeOf(n.Up.Limiters.Operations.Size).String()},
		{"up", "OpenTelemetry Metrics Enabled", n.Up.OtelMetrics, reflect.TypeOf(n.Up.OtelMetrics).String()},
		{"up", "OpenTelemetry Metrics Port", n.Up.OtelMetricsPort, reflect.TypeOf(n.Up.OtelMetricsPort).String()},
		{"up", "OpenTelemetry Metrics Exporter", n.Up.OtelMetricsExporter, reflect.TypeOf(n.Up.OtelMetricsExporter).String()},
		{"up", "OpenTelemetry Traces Enabled", n.Up.OtelTraces, reflect.TypeOf(n.Up.OtelTraces).String()},
		{"up", "OpenTelemetry Traces Exporter", n.Up.OtelTracesExporter, reflect.TypeOf(n.Up.OtelTracesExporter).String()},
		{"up", "OpenTelemetry Exporter URL", n.Up.OtlpExporterUrl, reflect.TypeOf(n.Up.OtlpExporterUrl).String()},
		{"preflight", "Force Dependency Install", n.Preflight.ForceDepInstall, reflect.TypeOf(n.Preflight.ForceDepInstall).String()},
	})
	fmt.Println(tw.Render())
	return nil
}

type PreflightCmd struct {
	ForceDepInstall bool `name:"force" default:"false" help:"Install missing dependencies without prompt" json:"preflight_force"`
}

func (p PreflightCmd) Run() error {
	return nil
}

func (u UpCmd) Run(ctx Context) error {
	return nil
}

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

// -----------------------------------------------------------------------
// Node configuration is used to configure the node process as well
// as the virtual machines it produces

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
