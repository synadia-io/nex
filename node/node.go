package node

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"slices"

	"ergo.services/application/observer"
	"ergo.services/ergo"
	"ergo.services/ergo/gen"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/node/actors"
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
	// ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	// defer cancel()

	// <-ctx.Done()
	nn.initializeSupervisionTree()
}

func (nn *nexNode) initializeSupervisionTree() {
	var options gen.NodeOptions

	// create applications that must be started
	apps := []gen.ApplicationBehavior{
		observer.CreateApp(observer.Options{}), // TODO: opt out of this via config
		actors.CreateNodeApp(),
	}
	options.Applications = apps

	// disable default logger to get rid of multiple logging to the os.Stdout
	options.Log.DefaultLogger.Disable = true

	// https://docs.ergo.services/basics/logging#process-logger
	// https://docs.ergo.services/tools/observer#log-process-page

	nodeName := "nex@localhost"

	// starting node
	node, err := ergo.StartNode(gen.Atom(nodeName), options)
	if err != nil {
		fmt.Printf("Unable to start node '%s': %s\n", nodeName, err)
		return
	}

	logger, err := node.Spawn(actors.CreateNodeLogger, gen.ProcessOptions{})
	if err != nil {
		panic(err) // TODO: no panic
	}

	// NOTE: the supervised processes won't log their startup (Init) calls because the
	// logger won't have been in place. However, they will log stuff afterward
	node.LoggerAddPID(logger, "nexlogger")

	node.Log().Info("Nex node started")
	node.Log().Info("Observer Application started and available at http://localhost:9911")
	node.Wait()
}
