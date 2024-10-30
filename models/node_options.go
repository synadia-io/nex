package models

import (
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
)

const (
	TagOS       = "nex.os"
	TagArch     = "nex.arch"
	TagCPUs     = "nex.cpucount"
	TagLameDuck = "nex.lameduck"
	TagNexus    = "nex.nexus"
)

var ReservedTagPrefixes = []string{"nex."}

type NodeOptions struct {
	Logger                *slog.Logger
	AgentHandshakeTimeout int
	ResourceDirectory     string
	Tags                  map[string]string
	ValidIssuers          []string
	OtelOptions           OTelOptions
	DisableDirectStart    bool
	AgentOptions          []AgentOptions
	HostServiceOptions    HostServiceOptions

	Errs error
}

type NodeOption func(*NodeOptions)

func WithLogger(s *slog.Logger) NodeOption {
	return func(n *NodeOptions) {
		if s != nil {
			n.Logger = s
		}
	}
}

func WithAgentHandshakeTimeout(t int) NodeOption {
	return func(n *NodeOptions) {
		n.AgentHandshakeTimeout = t
	}
}

func WithResourceDirectory(d string) NodeOption {
	return func(n *NodeOptions) {
		n.ResourceDirectory = d
	}
}

func WithNodeTags(t map[string]string) NodeOption {
	return func(n *NodeOptions) {
		for tag, v := range t {
			if slices.ContainsFunc(ReservedTagPrefixes,
				func(s string) bool {
					return strings.HasPrefix(tag, s)
				}) {
				n.Errs = errors.Join(n.Errs, errors.New("tag ["+tag+"] is using a reserved tag prefix"))
			} else {
				n.Tags[tag] = v
			}
		}
	}
}

func WithValidIssuers(v []string) NodeOption {
	return func(n *NodeOptions) {
		n.ValidIssuers = v
	}
}

func WithOTelOptions(o OTelOptions) NodeOption {
	return func(n *NodeOptions) {
		n.OtelOptions = o
	}
}

func WithDisableDirectStart(b bool) NodeOption {
	return func(n *NodeOptions) {
		n.DisableDirectStart = b
	}
}

func WithNexus(nexus string) NodeOption {
	return func(n *NodeOptions) {
		n.Tags["nex.nexus"] = nexus
	}
}

func WithExternalAgents(w []AgentOptions) NodeOption {
	return func(n *NodeOptions) {
		n.AgentOptions = w
	}
}

func WithHostServiceOptions(h HostServiceOptions) NodeOption {
	return func(n *NodeOptions) {
		n.HostServiceOptions = h
	}
}

type OTelOptions struct {
	MetricsEnabled   bool
	MetricsExporter  string
	MetricsPort      int
	TracesEnabled    bool
	TracesExporter   string
	ExporterEndpoint string
}

type AgentOptions struct {
	Name          string
	Uri           string
	Configuration json.RawMessage
}

type HostServiceOptions struct {
	NatsUrl      string
	NatsUserJwt  string
	NatsUserSeed string
	Services     map[string]ServiceConfig
}

type ServiceConfig struct {
	Enabled       bool
	Configuration json.RawMessage
}
