package models

import (
	"encoding/json"
	"log/slog"
)

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
		n.Tags = t
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
