package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/nats-io/nkeys"

	"github.com/synadia-io/nex/node"
)

type Node struct {
	Up        Up        `cmd:"" help:"Bring a node up"`
	Preflight Preflight `cmd:"" help:"Run a preflight check on a node" aliases:"init"`
	LameDuck  LameDuck  `cmd:"" name:"lameduck" help:"Command a node to enter lame duck mode" aliases:"down"`
	List      List      `cmd:"" aliases:"ls" help:"List running nodes"`
	Info      Info      `cmd:"" help:"Provide information about a running node"`
}

// ----- Preflight Command -----
type Preflight struct {
	Force          bool     `optional:"" help:"Force the preflight check to run. All artifacts are (re-)installed"`
	Yes            bool     `optional:"" short:"y" help:"Answer yes to all prompts"`
	GenConfig      bool     `optional:"" help:"Creates a default configuration file in your current directory"`
	Status         bool     `optional:"" help:"Check the status of the node requirements without installing anything"`
	InstallVersion string   `optional:"" help:"Bypasses checking 'latest' and installs a specific version of NEX.  Must be valid release tag in Github" placeholder:"v0.3.0"`
	CniNS          []string `optional:"" help:"The DNS nameservers to add to CNI config.  If blank, MicroVMs will not be able to do DNS lookups" placeholder:"1.1.1.1"`
	GithubPAT      string   `optional:"" help:"GitHub Personal Access Token. Can be provided if rate limits are hit pulling data from Github" placeholder:"ghp_abc123..."`
}

func (p Preflight) Validate() error {
	var errs error
	if p.InstallVersion != "" {
		url := "https://api.github.com/repos/synadia-io/nex/releases"

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}

		if p.GithubPAT != "" {
			req.Header.Set("Authorization", "token "+p.GithubPAT)
		}

		req.Header.Set("Accept", "application/vnd.github.v3+json")

		client := &http.Client{
			Timeout: 5 * time.Second,
		}

		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GitHub API request failed with status: %s", resp.Status)
		}

		type GitHubRelease struct {
			TagName string `json:"tag_name"`
		}
		var releases []GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			return err
		}

		for _, r := range releases {
			if r.TagName == p.InstallVersion {
				break
			}
		}
		return fmt.Errorf("Did not find prefered install version in Github")
	}

	if len(p.CniNS) > 0 {
		for _, ns := range p.CniNS {
			if ip := net.ParseIP(ns); ip == nil {
				errs = errors.Join(errs, fmt.Errorf("invalid IP address provided to CNI Nameserver setting: %s", ns))
			}
		}
	}

	return errs
}

func (p Preflight) Run(ctx context.Context, globals Globals) error {
	if globals.Check {
		return printTable("Node Preflight Configuration", append(globals.Table(), p.Table()...)...)
	}
	fmt.Println("run preflight")
	return nil
}

// ----- LameDuck Command -----
type LameDuck struct {
	NodeID string            `optional:"" help:"Node ID to command into lame duck mode" placeholder:"NBTAFHAKW..."`
	Label  map[string]string `optional:"" help:"Put all nodes with label in lameduck.  Only 1 label allowed" placeholder:"nex.nexus=mynexus"`
}

func (l LameDuck) Validate() error {
	if l.NodeID == "" && len(l.Label) == 0 {
		return errors.New("must provide a node ID or label")
	}
	if l.NodeID != "" && !nkeys.IsValidPublicServerKey(l.NodeID) {
		return errors.New("invalid node ID provided")
	}
	if len(l.Label) > 1 {
		return errors.New("only one label allowed")
	}

	switch {
	case l.NodeID != "":
		fmt.Println("Putting node in lameduck")
	case len(l.Label) == 1:
		fmt.Println("Putting all nodes with label in lameduck")
	default:
		return errors.New("used must provide valid Node ID or one label")
	}

	return nil
}

func (l LameDuck) Run(ctx context.Context, globals Globals) error {
	if globals.Check {
		return printTable("Node Lameduck Configuration", append(globals.Table(), l.Table()...)...)
	}
	fmt.Println("run lameduck")
	return nil
}

// ----- List Command -----
type List struct {
	Filter map[string]string `optional:"" help:"Filter the list of nodes on tags" placeholder:"nex.nexus=mynexus;..."`
	JSON   bool              `optional:"" help:"Output in JSON format"`
}

func (l List) Validate() error {
	return nil
}

func (l List) Run(ctx context.Context, globals Globals) error {
	if globals.Check {
		return printTable("Node List Configuration", append(globals.Table(), l.Table()...)...)
	}
	fmt.Println("run list")
	return nil
}

// ----- Info Command -----
type Info struct {
	NodeID string `arg:"" required:"" help:"Node ID to query" placeholder:"NBTAFHAKW..."`
	JSON   bool   `optional:"" help:"Output in JSON format"`
}

func (i Info) Validate() error {
	var errs error
	if !nkeys.IsValidPublicServerKey(i.NodeID) {
		errs = errors.Join(errs, errors.New("invalid node ID provided"))
	}
	return errs
}

func (i Info) Run(ctx context.Context, globals Globals) error {
	if globals.Check {
		return printTable("Node Info Configuration", append(globals.Table(), i.Table()...)...)
	}
	fmt.Println("run info")
	return nil
}

// ----- Up Command -----
type Up struct {
	AgentHandshakeTimeoutMillisecond int               `default:"5000" group:"Nex Node Up Configuration" json:"up_agent_handshake_timeout_ms"`
	DefaultResourceDir               string            `default:"./resources" group:"Nex Node Up Configuration" json:"up_default_resource_dir"`
	FirecrackerBinPath               string            `default:"/usr/local/bin/firecracker" group:"Nex Node Up Configuration" json:"up_firecracker_bin"`
	InternalNodeHost                 string            `default:"192.168.127.1" group:"Nex Node Up Configuration" json:"up_internal_node_host"`
	InternalNodePort                 int               `default:"9222" group:"Nex Node Up Configuration" json:"up_internal_node_port"`
	KernelFilepath                   string            `group:"Nex Node Up Configuration" json:"up_kernel_filepath"`
	MachinePoolSize                  int               `default:"1" group:"Nex Node Up Configuration" json:"up_machine_pool_size"`
	MicroVM                          bool              `default:"false" group:"Nex Node Up Configuration" json:"up_microvm"`
	NexusName                        string            `optional:"" help:"Nexus name" default:"nexus"`
	PreserveNetwork                  bool              `default:"true" group:"Nex Node Up Configuration" json:"up_preserve_network"`
	PublicNATSServer                 []byte            `placeholder:"./path/to/nats_server.conf" type:"filecontent" help:"Path to nats server config to be used for userland NATS server. JSON format" group:"Nex Node Up Configuration"`
	RootFsFilepath                   string            `group:"Nex Node Up Configuration" json:"up_rootfs_filepath"`
	Tags                             map[string]string `placeholder:"nex:iscool;..." group:"Nex Node Up Configuration" json:"up_tags"`
	ValidIssuers                     []string          `group:"Nex Node Up Configuration" json:"up_valid_issuers"`
	//WorkloadTypes                    []models.NexWorkload `default:"native" enum:"native,function,container,wasm" group:"Nex Node Up Configuration" json:"up_workload_types"`

	CNIDefinition      *CNIDefinition      `embed:"" group:"CNI Configuration"`
	MachineTemplate    *MachineTemplate    `embed:"" group:"Firecracker Machine Configuration"`
	Limiters           *Limiters           `embed:"" prefix:"limiter_" group:"Limiter Configuration"`
	HostServicesConfig *HostServicesConfig `embed:"" group:"Host Services Configuration"`
	OtelConfig         *OtelConfig         `embed:"" group:"OpenTelemetry Configuration"`
}

func (u Up) Validate() error {
	return nil
}

func (u Up) Run(ctx context.Context, globals Globals) error {
	if globals.Check {
		return printTable("Node Up Configuration", append(globals.Table(), u.Table()...)...)
	}

	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	kp, err := nkeys.CreateServer()
	if err != nil {
		return err
	}

	pubKey, err := kp.PublicKey()
	if err != nil {
		return err
	}

	logger := configureLogger(globals, nc, pubKey)

	nexNode, err := node.NewNexNode(nc,
		node.WithLogger(logger),
		node.WithAgentHandshakeTimeout(u.AgentHandshakeTimeoutMillisecond),
		node.WithResourceDirectory(u.DefaultResourceDir),
		node.WithInternalNodeNATHost(u.InternalNodeHost, u.InternalNodePort),
		node.WithMicroVMMode(u.MicroVM),
		node.WithPreserveNetwork(u.PreserveNetwork),
		node.WithKernelFilepath(u.KernelFilepath),
		node.WithRootFsFilepath(u.RootFsFilepath),
		node.WithNodeTags(u.Tags),
		node.WithValidIssuers(u.ValidIssuers),
		node.WithCNIOptions(node.CNIOptions{
			BinPaths:      u.CNIDefinition.CniBinPaths,
			InterfaceName: u.CNIDefinition.CniInterfaceName,
			NetworkName:   u.CNIDefinition.CniNetworkName,
			Subnet:        u.CNIDefinition.CniSubnet,
		}),
		node.WithFirecrackerOptions(node.FirecrackerOptions{
			VcpuCount: u.MachineTemplate.FirecrackerVcpuCount,
			MemoryMiB: u.MachineTemplate.FirecrackerMemSizeMib,
		}),
		node.WithBandwidthOptions(node.BandwithOptions{
			OneTimeBurst: u.Limiters.Bandwidth.OneTimeBurst,
			RefillTime:   u.Limiters.Bandwidth.RefillTime,
			Size:         u.Limiters.Bandwidth.Size,
		}),
		node.WithOperationsOptions(node.OperationsOptions{
			OneTimeBurst: u.Limiters.Operations.OneTimeBurst,
			RefillTime:   u.Limiters.Operations.RefillTime,
			Size:         u.Limiters.Operations.Size,
		}),
		node.WithOTelOptions(node.OTelOptions{
			MetricsEnabled:   u.OtelConfig.OtelMetrics,
			MetricsPort:      u.OtelConfig.OtelMetricsPort,
			MetricsExporter:  u.OtelConfig.OtelMetricsExporter,
			TracesEnabled:    u.OtelConfig.OtelTraces,
			TracesExporter:   u.OtelConfig.OtelTracesExporter,
			ExporterEndpoint: u.OtelConfig.OtlpExporterUrl,
		}),
	)
	if err != nil {
		return err
	}

	err = nexNode.Validate()
	if err != nil {
		return err
	}

	logger.Info("Starting Nex Node")
	nexNode.Start()
	logger.Info("Shutting down Nex Node")

	return nil
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
