package node

import (
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
		n.internalNodeNATHost = h
		n.internalNodeNATPort = p
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

type OTelOptions struct {
	MetricsEnabled   bool
	MetricsExporter  string
	MetricsPort      int
	TracesEnabled    bool
	TracesExporter   string
	ExporterEndpoint string
}
