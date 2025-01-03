package models

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"

	"github.com/nats-io/nkeys"
)

type NodeOptions struct {
	Context               context.Context
	Logger                *slog.Logger
	Xkey                  nkeys.KeyPair
	AgentHandshakeTimeout int
	ResourceDirectory     string
	Tags                  map[string]string
	ValidIssuers          []string
	OtelOptions           OTelOptions
	DisableDirectStart    bool
	AgentOptions          []AgentOptions
	HostServiceOptions    HostServiceOptions
	OCICacheRegistry      string
	DevMode               bool

	StartWorkloadMessage string
	StopWorkloadMessage  string

	Errs error
}

type NodeOption func(*NodeOptions)

func WithContext(ctx context.Context) NodeOption {
	return func(n *NodeOptions) {
		n.Context = ctx
	}
}

func WithLogger(s *slog.Logger) NodeOption {
	return func(n *NodeOptions) {
		if s != nil {
			n.Logger = s
		}
	}
}

func WithXKeyKeyPair(kp nkeys.KeyPair) NodeOption {
	return func(n *NodeOptions) {
		n.Xkey = kp
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
		if nexus != "" {
			n.Tags[TagNexus] = nexus
		}
	}
}

func WithNodeName(name string) NodeOption {
	return func(n *NodeOptions) {
		if name != "" {
			n.Tags[TagNodeName] = name
		}
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

func WithOCICacheRegistry(r string) NodeOption {
	return func(n *NodeOptions) {
		n.OCICacheRegistry = r
	}
}

func WithDevMode(b bool) NodeOption {
	return func(n *NodeOptions) {
		n.DevMode = b
	}
}

func WithStartWorkloadMessage(s string) NodeOption {
	return func(n *NodeOptions) {
		n.StartWorkloadMessage = s
	}
}

func WithStopWorkloadMessage(s string) NodeOption {
	return func(n *NodeOptions) {
		n.StopWorkloadMessage = s
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
