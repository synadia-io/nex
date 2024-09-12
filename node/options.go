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

func WithMicroVMMode(m bool) NexOption {
	return func(n *nexNode) {
		n.microVMMode = m
	}
}

func WithPreserveNetwork(p bool) NexOption {
	return func(n *nexNode) {
		n.preserveNetwork = p
	}
}

func WithKernelFilepath(k string) NexOption {
	return func(n *nexNode) {
		n.kernelFilepath = k
	}
}

func WithRootFsFilepath(r string) NexOption {
	return func(n *nexNode) {
		n.rootFsFilepath = r
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

func WithCNIOptions(c CNIOptions) NexOption {
	return func(n *nexNode) {
		n.cniOptions = c
	}
}

func WithFirecrackerOptions(f FirecrackerOptions) NexOption {
	return func(n *nexNode) {
		n.firecrackerOptions = f
	}
}

func WithBandwidthOptions(b BandwithOptions) NexOption {
	return func(n *nexNode) {
		n.bandwidthOptions = b
	}
}

func WithOperationsOptions(o OperationsOptions) NexOption {
	return func(n *nexNode) {
		n.operationsOptions = o
	}
}

func WithOTelOptions(o OTelOptions) NexOption {
	return func(n *nexNode) {
		n.otelOptions = o
	}
}

type CNIOptions struct {
	BinPaths      []string
	InterfaceName string
	NetworkName   string
	Subnet        string
}

type FirecrackerOptions struct {
	VcpuCount int
	MemoryMiB int
}

type BandwithOptions struct {
	OneTimeBurst int64
	RefillTime   int64
	Size         int64
}

type OperationsOptions struct {
	OneTimeBurst int64
	RefillTime   int64
	Size         int64
}

type OTelOptions struct {
	MetricsEnabled   bool
	MetricsExporter  string
	MetricsPort      int
	TracesEnabled    bool
	TracesExporter   string
	ExporterEndpoint string
}
