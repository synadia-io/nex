package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/nats-io/nkeys"

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
	AgentHandshakeTimeoutMillisecond int               `help:"Timeout in milliseconds" name:"agent-timeout" default:"5000"`
	DefaultResourceDir               string            `default:"${defaultResourcePath}"`
	NexusName                        string            `default:"nexus" help:"Nexus name"`
	Tags                             map[string]string `placeholder:"nex:iscool;..." help:"Tags to be used for nex node"`
	ValidIssuers                     []string          `placeholder:"NBTAFHAKW..." help:"List of valid issuers for public nkey"`
	WorkloadTypes                    WorkloadConfigs   `help:"Workload types configurations for nex node to initialize"`

	HostServicesConfig HostServicesConfig `embed:"" prefix:"hostservices." group:"Host Services Configuration"`
	OtelConfig         OtelConfig         `embed:"" prefix:"otel." group:"OpenTelemetry Configuration"`
}

func (u Up) Validate() error {
	var errs error
	if u.WorkloadTypes == nil || len(u.WorkloadTypes) < 1 {
		errs = errors.Join(errs, errors.New("attempting to start nex node with no workload types configured. Please provide at least 1 workload type configuration"))
	}
	return errs
}

func (u Up) Run(ctx context.Context, globals Globals, n *Node) error {
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
		options.WithLogger(logger),
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
		options.WithWorkloadTypes(func() []options.WorkloadOptions {
			ret := make([]options.WorkloadOptions, len(u.WorkloadTypes))
			for i, e := range u.WorkloadTypes {
				ret[i] = options.WorkloadOptions{
					Name:     e.Name,
					AgentUri: e.AgentUri,
					Argv:     e.Argv,
					Env:      e.Env,
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

	logger.Info("Starting Nex Node")
	err = nexNode.Start() // As this is a blocking call, it should return when the node is shutting down
	if err != nil {
		logger.Error("Failed to start Nex Node", slog.String("error", err.Error()))
	}
	logger.Info("Shutting down Nex Node")

	return nil
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
	Enabled       bool            `group:"Services Configuration"`
	Configuration json.RawMessage `group:"Services Configuration"`
}

type WorkloadConfig struct {
	Name     string            `help:"Name of the workload type" placeholder:"javascript"`
	AgentUri string            `name:"agenturi" help:"URI to the agent binary to download and install in resource directory" placeholder:"nats://bucket/key"`
	Argv     []string          `help:"Arguments to pass to the agent. Comma seperated" placeholder:"--foo=bar,--true"`
	Env      map[string]string `help:"Environment variables to pass to the agent" placeholder:"NAME=derp"`
}
type WorkloadConfigs []WorkloadConfig