package node

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats.go"
)

type Node interface {
	Validate() error
	Start()
}

type nexNode struct {
	nc *nats.Conn

	logger                *slog.Logger
	agentHandshakeTimeout int
	resourceDirectory     string
	internalNodeNATHost   string
	internalNodeNATPort   int
	microVMMode           bool
	preserveNetwork       bool
	kernelFilepath        string
	rootFsFilepath        string
	tags                  map[string]string
	validIssuers          []string
	cniOptions            CNIOptions
	firecrackerOptions    FirecrackerOptions
	bandwidthOptions      BandwithOptions
	operationsOptions     OperationsOptions
	otelOptions           OTelOptions
}

type NexOption func(*nexNode)

func NewNexNode(nc *nats.Conn, opts ...NexOption) (Node, error) {
	if nc == nil {
		return nil, fmt.Errorf("no nats connection provided")
	}

	nn := &nexNode{
		nc:                    nc,
		logger:                slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})),
		agentHandshakeTimeout: 5000,
		resourceDirectory:     "./resources",
		internalNodeNATHost:   nats.DefaultURL,
		internalNodeNATPort:   4222,
		microVMMode:           false,
		preserveNetwork:       true,
		tags:                  make(map[string]string),
		validIssuers:          []string{},
		cniOptions: CNIOptions{
			BinPaths:      []string{"/opt/cni/bin"},
			InterfaceName: "veth0",
			NetworkName:   "fcnet",
			Subnet:        "192.168.127.0/24",
		},
		firecrackerOptions: FirecrackerOptions{
			VcpuCount: 1,
			MemoryMiB: 256,
		},
		bandwidthOptions: BandwithOptions{
			OneTimeBurst: 0,
			RefillTime:   0,
			Size:         0,
		},
		operationsOptions: OperationsOptions{
			OneTimeBurst: 0,
			RefillTime:   0,
			Size:         0,
		},
		otelOptions: OTelOptions{
			MetricsEnabled:   false,
			MetricsPort:      8085,
			MetricsExporter:  "file",
			TracesEnabled:    false,
			TracesExporter:   "file",
			ExporterEndpoint: "127.0.0.1:14532",
		},
	}

	for _, opt := range opts {
		opt(nn)
	}

	return nn, nil
}

func (nn *nexNode) Validate() error {
	var errs error

	return errs
}

func (nn *nexNode) Start() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	<-ctx.Done()
}
