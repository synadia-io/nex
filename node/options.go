package node

import (
	"encoding/json"
	"log/slog"
)

func WithLogger(s *slog.Logger) NexOption {
	return func(n *nexNode) {
		if s != nil {
			n.logger = s
		}
	}
}

func WithAgentHandshakeTimeout(t int) NexOption {
	return func(n *nexNode) {
		n.agentHandshakeTimeout = t
	}
}

func WithResourceDirectory(d string) NexOption {
	return func(n *nexNode) {
		n.resourceDirectory = d
	}
}

func WithInternalNodeNATHost(h string, p int) NexOption {
	return func(n *nexNode) {
		n.internalNodeNATSHost = h
		n.internalNodeNATSPort = p
	}
}

func WithNodeTags(t map[string]string) NexOption {
	return func(n *nexNode) {
		n.tags = t
	}
}

func WithValidIssuers(v []string) NexOption {
	return func(n *nexNode) {
		n.validIssuers = v
	}
}

func WithOTelOptions(o OTelOptions) NexOption {
	return func(n *nexNode) {
		n.otelOptions = o
	}
}

func WithWorkloadTypes(w []WorkloadOptions) NexOption {
	return func(n *nexNode) {
		n.workloadOptions = w
	}
}

func WithHostServiceOptions(h HostServiceOptions) NexOption {
	return func(n *nexNode) {
		n.hostServiceOptions = h
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

type WorkloadOptions struct {
	Name     string
	AgentUri string
	Argv     []string
	Env      map[string]string
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
