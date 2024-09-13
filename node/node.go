package node

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
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
	internalNodeNATSHost  string
	internalNodeNATSPort  int
	tags                  map[string]string
	validIssuers          []string
	otelOptions           OTelOptions
	workloadOptions       []WorkloadOptions
	hostServiceOptions    HostServiceOptions
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
		internalNodeNATSHost:  "127.0.0.1",
		internalNodeNATSPort:  -1,
		tags:                  make(map[string]string),
		validIssuers:          []string{},
		otelOptions: OTelOptions{
			MetricsEnabled:   false,
			MetricsPort:      8085,
			MetricsExporter:  "file",
			TracesEnabled:    false,
			TracesExporter:   "file",
			ExporterEndpoint: "127.0.0.1:14532",
		},
		workloadOptions: []WorkloadOptions{},
		hostServiceOptions: HostServiceOptions{
			Services: make(map[string]ServiceConfig),
		},
	}

	for _, opt := range opts {
		opt(nn)
	}

	return nn, nil
}

func (nn *nexNode) Validate() error {
	var errs error

	if nn.nc == nil {
		errs = errors.Join(errs, errors.New("nats connection is nil"))
	}

	if nn.logger == nil {
		errs = errors.Join(errs, errors.New("logger is nil"))
	}

	if nn.agentHandshakeTimeout <= 0 {
		errs = errors.Join(errs, errors.New("agent handshake timeout must be greater than 0"))
	}

	if len(nn.workloadOptions) <= 0 {
		errs = errors.Join(errs, errors.New("node required at least 1 workload type be configured in order to start"))
	}

	if nn.resourceDirectory != "" {
		if _, err := os.Stat(nn.resourceDirectory); os.IsNotExist(err) {
			errs = errors.Join(errs, errors.New("resource directory does not exist"))
		}
	}

	for _, vi := range nn.validIssuers {
		if !nkeys.IsValidPublicServerKey(vi) {
			errs = errors.Join(errs, errors.New("invalid issuer public key: "+vi))
		}
	}

	if nn.otelOptions.MetricsEnabled {
		if nn.otelOptions.MetricsPort <= 0 || nn.otelOptions.MetricsPort > 65535 {
			errs = errors.Join(errs, errors.New("invalid metrics port"))
		}
		if nn.otelOptions.MetricsExporter == "" || !slices.Contains([]string{"file", "prometheus"}, nn.otelOptions.MetricsExporter) {
			errs = errors.Join(errs, errors.New("invalid metrics exporter"))
		}
	}

	if nn.otelOptions.TracesEnabled {
		if nn.otelOptions.TracesExporter == "" || !slices.Contains([]string{"file", "http", "grpc"}, nn.otelOptions.TracesExporter) {
			errs = errors.Join(errs, errors.New("invalid traces exporter"))
		}
		if nn.otelOptions.TracesExporter == "http" || nn.otelOptions.TracesExporter == "grpc" {
			if _, err := url.Parse(nn.otelOptions.ExporterEndpoint); err != nil {
				errs = errors.Join(errs, errors.New("invalid traces exporter endpoint"))
			}
		}
	}

	return errs
}

func (nn *nexNode) Start() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	<-ctx.Done()
}
