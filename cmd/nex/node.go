package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/nats-io/nkeys"

	"github.com/synadia-io/nex/api/nodecontrol"
	"github.com/synadia-io/nex/models"
	options "github.com/synadia-io/nex/models"
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
	Force          bool   `optional:"" help:"Force the preflight check to run. All artifacts are (re-)installed"`
	Yes            bool   `optional:"" short:"y" help:"Answer yes to all prompts"`
	GenConfig      bool   `optional:"" help:"Creates a default configuration file in your current directory"`
	Status         bool   `optional:"" help:"Check the status of the node requirements without installing anything"`
	InstallVersion string `optional:"" help:"Bypasses checking 'latest' and installs a specific version of NEX.  Must be valid release tag in Github" placeholder:"v0.3.0"`
	GithubPAT      string `optional:"" help:"GitHub Personal Access Token. Can be provided if rate limits are hit pulling data from Github" placeholder:"ghp_abc123..."`
}

func (Preflight) AfterApply(globals *Globals) error {
	return checkVer(globals)
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

	return errs
}

func (p Preflight) Run(ctx context.Context, globals *Globals) error {
	if globals.Check {
		return printTable("Node Preflight Configuration", append(globals.Table(), p.Table()...)...)
	}
	return nil
}

// ----- LameDuck Command -----
type LameDuck struct {
	NodeID string            `optional:"" help:"Node ID to command into lame duck mode" placeholder:"NBTAFHAKW..."`
	Label  map[string]string `optional:"" help:"Put all nodes with label in lameduck.  Only 1 label allowed" placeholder:"nex.nexus=mynexus"`
}

func (LameDuck) AfterApply(globals *Globals) error {
	return checkVer(globals)
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

func (l LameDuck) Run(ctx context.Context, globals *Globals) error {
	if globals.Check {
		return printTable("Node Lameduck Configuration", append(globals.Table(), l.Table()...)...)
	}
	fmt.Println("run lameduck")
	return nil
}

// ----- List Command -----
type List struct {
	Filter map[string]string `optional:"" help:"Filter the list of nodes on tags. Node must match all provided tags to be returned" placeholder:"nex.nexus=mynexus"`
	JSON   bool              `optional:"" help:"Output in JSON format"`
}

func (List) AfterApply(globals *Globals) error {
	return checkVer(globals)
}

func (l List) Validate() error {
	return nil
}

func (l List) Run(ctx context.Context, globals *Globals) error {
	if globals.Check {
		return printTable("Node List Configuration", append(globals.Table(), l.Table()...)...)
	}
	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	controller, err := nodecontrol.NewControlApiClient(nc, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err != nil {
		return err
	}

	resp, err := controller.Ping()
	if err != nil {
		return err
	}

	if !l.JSON {
		tW := newTableWriter("NATS Execution Nodes")
		tW.AppendHeader(table.Row{"Nexus", "ID (* = Lameduck Mode)", "Name", "Version", "Agents", "Workloads", "Uptime", "OS", "Arch"})

	node_loop:
		for _, n := range resp {

			// Check the filter list
			for k, v := range l.Filter {
				if tV, ok := n.Tags.Tags[k]; !ok || tV != v {
					continue node_loop
				}
			}

			lameduck, ok := n.Tags.Tags[models.TagLameDuck]
			if !ok || strings.ToLower(lameduck) != "true" {
				lameduck = "false"
			}

			tW.AppendRow(table.Row{
				n.Nexus,
				func() string {
					if lameduck == "true" {
						return n.NodeId + "*"
					}
					return n.NodeId
				}(),
				n.Tags.Tags[models.TagNodeName],
				n.Version,
				len(n.RunningAgents.Status),
				func() string {
					count := 0
					for _, v := range n.RunningAgents.Status {
						count += v
					}
					return strconv.Itoa(count)
				}(),
				n.Uptime,
				n.Tags.Tags[models.TagOS],
				n.Tags.Tags[models.TagArch],
			})
		}
		fmt.Println(tW.Render())
		return nil
	}

	resp_b, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	fmt.Println(string(resp_b))
	return nil
}

// ----- Info Command -----
type Info struct {
	NodeID string `arg:"" required:"" help:"Node ID to query" placeholder:"NBTAFHAKW..."`
	JSON   bool   `optional:"" help:"Output in JSON format"`
}

func (Info) AfterApply(globals *Globals) error {
	return checkVer(globals)
}

func (i Info) Validate() error {
	var errs error
	if !nkeys.IsValidPublicServerKey(i.NodeID) {
		errs = errors.Join(errs, errors.New("invalid node ID provided"))
	}
	return errs
}

func (i Info) Run(ctx context.Context, globals *Globals) error {
	if globals.Check {
		return printTable("Node Info Configuration", append(globals.Table(), i.Table()...)...)
	}
	fmt.Println("run info")
	return nil
}

// ----- Up Command -----
type Up struct {
	AgentHandshakeTimeoutMillisecond int               `help:"Timeout in milliseconds" name:"agent-timeout" default:"5000"`
	DefaultResourceDir               string            `name:"resource-directory" default:"${defaultResourcePath}"`
	NexusName                        string            `name:"nexus" default:"nexus" help:"Nexus name"`
	NodeName                         string            `placeholder:"nex-node" help:"Name of the node; random if not provided"`
	Tags                             map[string]string `placeholder:"nex:iscool;..." help:"Tags to be used for nex node"`
	ValidIssuers                     []string          `placeholder:"NBTAFHAKW..." help:"List of valid issuers for public nkey"`
	Agents                           AgentConfigs      `help:"Workload types configurations for nex node to initialize"`
	DisableDirectStart               bool              `help:"Disable direct start (no sandbox) workloads" default:"false"`
	NodeSeed                         string            `help:"Node Seed used for identifier.  Default is generated" placeholder:"NBTAFHAKW..."`
	HideWorkloadLogs                 bool              `name:"hide-workload-logs" help:"Hide logs from workloads" default:"false"`
	ShowSystemLogs                   bool              `name:"system-logs" help:"Show verbose level logs from inside actor framework" default:"false" hidden:""`

	HostServicesConfig HostServicesConfig `embed:"" prefix:"hostservices." group:"Host Services Configuration"`
	OtelConfig         OtelConfig         `embed:"" prefix:"otel." group:"OpenTelemetry Configuration"`
}

func (u *Up) AfterApply(globals *Globals) error {
	return checkVer(globals)
}

func (u Up) Validate() error {
	var errs error
	if len(u.Agents) < 1 && u.DisableDirectStart {
		errs = errors.Join(errs, errors.New("attempting to start nex node with no workload types configured. Please provide at least 1 workload type configuration or enable direct start"))
	}

	if u.NodeSeed != "" {
		prefix, _, err := nkeys.DecodeSeed([]byte(u.NodeSeed))
		if err != nil || prefix != nkeys.PrefixByteServer {
			errs = errors.Join(errs, errors.New("invalid node seed provided"))
		}
	}

	return errs
}

func (u Up) Run(ctx context.Context, globals *Globals, n *Node) error {
	if globals.Check {
		return printTable("Node Up Configuration", append(globals.Table(), u.Table()...)...)
	}

	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	var kp nkeys.KeyPair
	if u.NodeSeed == "" {
		kp, err = nkeys.CreateServer()
		if err != nil {
			return err
		}
	} else {
		kp, err = nkeys.FromSeed([]byte(u.NodeSeed))
		if err != nil {
			return err
		}
	}

	pubKey, err := kp.PublicKey()
	if err != nil {
		return err
	}

	logger := configureLogger(globals, nc, pubKey, u.ShowSystemLogs, u.HideWorkloadLogs)

	nexNode, err := node.NewNexNode(kp, nc,
		options.WithLogger(logger),
		options.WithNodeName(u.NodeName),
		options.WithNexus(u.NexusName),
		options.WithAgentHandshakeTimeout(u.AgentHandshakeTimeoutMillisecond),
		options.WithResourceDirectory(u.DefaultResourceDir),
		options.WithNodeTags(u.Tags),
		options.WithValidIssuers(u.ValidIssuers),
		options.WithOTelOptions(options.OTelOptions{
			MetricsEnabled:   u.OtelConfig.OtelMetrics,
			MetricsPort:      u.OtelConfig.OtelMetricsPort,
			MetricsExporter:  u.OtelConfig.OtelMetricsExporter,
			TracesEnabled:    u.OtelConfig.OtelTraces,
			TracesExporter:   u.OtelConfig.OtelTracesExporter,
			ExporterEndpoint: u.OtelConfig.OtlpExporterUrl,
		}),
		options.WithDisableDirectStart(false),
		options.WithExternalAgents(func() []options.AgentOptions {
			ret := make([]options.AgentOptions, len(u.Agents))
			for i, e := range u.Agents {
				ret[i] = options.AgentOptions{
					Name:          e.Name,
					Uri:           e.AgentUri,
					Configuration: e.Configuration,
				}
			}
			return ret
		}()),
		options.WithHostServiceOptions(func() options.HostServiceOptions {
			return options.HostServiceOptions{
				NatsUrl:      u.HostServicesConfig.NatsUrl,
				NatsUserJwt:  u.HostServicesConfig.NatsUserJwt,
				NatsUserSeed: u.HostServicesConfig.NatsUserSeed,
				Services: func() map[string]options.ServiceConfig {
					ret := make(map[string]options.ServiceConfig, len(u.HostServicesConfig.Services))
					for k, v := range u.HostServicesConfig.Services {
						ret[k] = options.ServiceConfig{
							Enabled:       v.Enabled,
							Configuration: v.Configuration,
						}
					}
					return ret
				}(),
			}
		}()),
	)
	if err != nil {
		return err
	}

	logger.Info("Validating Nex Node")
	err = nexNode.Validate()
	if err != nil {
		return err
	}

	logger.Info("Starting Nex Node", slog.String("public_key", pubKey), slog.String("config", string(globals.Config)))
	return nexNode.Start() // As this is a blocking call, it should return only when the node is shutting down
}

type OtelConfig struct {
	OtelMetrics         bool   `name:"metrics" default:"false" help:"Enables OTel Metrics"`
	OtelMetricsPort     int    `name:"metrics-port" default:"8085"`
	OtelMetricsExporter string `name:"metrics-exporter" default:"file" enum:"file,prometheus"`
	OtelTraces          bool   `name:"traces" default:"false" help:"Enables OTel Traces"`
	OtelTracesExporter  string `name:"traces-exporter" default:"file" enum:"file,grpc,http"`
	OtlpExporterUrl     string `name:"exporter-url" default:"127.0.0.1:14532"`
}

type HostServicesConfig struct {
	NatsUrl      string                   `group:"Host Services Configuration"`
	NatsUserJwt  string                   `group:"Host Services Configuration"`
	NatsUserSeed string                   `group:"Host Services Configuration"`
	Services     map[string]ServiceConfig `prefix:"service." group:"Host Services Configuration"`
}

type ServiceConfig struct {
	Enabled       bool            `default:"false"`
	Configuration json.RawMessage `placeholder:"{}"`
}

type AgentConfig struct {
	Name          string          `help:"Name of the workload type" placeholder:"javascript"`
	AgentUri      string          `name:"agenturi" help:"URI to the agent binary to download and install in resource directory" placeholder:"nats://bucket/key"`
	Configuration json.RawMessage `help:"Configuration JSON for agent" placeholder:"{}"`
}
type AgentConfigs []AgentConfig
