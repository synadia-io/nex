package node

import (
	"context"
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
	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/nex/node/actors"
)

type Node interface {
	Validate() error
	Start() error
}

type nexNode struct {
	nc  *nats.Conn
	ctx context.Context

	options   *models.NodeOptions
	publicKey nkeys.KeyPair
}

func NewNexNode(publicKey nkeys.KeyPair, nc *nats.Conn, opts ...models.NodeOption) (Node, error) {
	if nc == nil {
		return nil, fmt.Errorf("no nats connection provided")
	}

	nn := &nexNode{
		nc:  nc,
		ctx: context.Background(),
		options: &models.NodeOptions{
			Logger:                slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})),
			AgentHandshakeTimeout: 5000,
			ResourceDirectory:     "./resources",
			Tags:                  make(map[string]string),
			ValidIssuers:          []string{},
			OtelOptions: models.OTelOptions{
				MetricsEnabled:   false,
				MetricsPort:      8085,
				MetricsExporter:  "file",
				TracesEnabled:    false,
				TracesExporter:   "file",
				ExporterEndpoint: "127.0.0.1:14532",
			},
			WorkloadOptions: []models.WorkloadOptions{},
			HostServiceOptions: models.HostServiceOptions{
				Services: make(map[string]models.ServiceConfig),
			},
		},
		publicKey: publicKey,
	}

	if len(opts) > 0 && opts[0] != nil {
		for _, opt := range opts {
			opt(nn.options)
		}
	}

	err := nn.Validate()
	if err != nil {
		return nil, err
	}

	return nn, nil
}

func (nn *nexNode) Validate() error {
	var errs error

	if nn.options.Logger == nil {
		errs = errors.Join(errs, errors.New("logger is nil"))
	}

	if nn.options.AgentHandshakeTimeout <= 0 {
		errs = errors.Join(errs, errors.New("agent handshake timeout must be greater than 0"))
	}

	if len(nn.options.WorkloadOptions) <= 0 {
		errs = errors.Join(errs, errors.New("node required at least 1 workload type be configured in order to start"))
	}

	if nn.options.ResourceDirectory != "" {
		if _, err := os.Stat(nn.options.ResourceDirectory); os.IsNotExist(err) {
			errs = errors.Join(errs, errors.New("resource directory does not exist"))
		}
	}

	for _, vi := range nn.options.ValidIssuers {
		if !nkeys.IsValidPublicServerKey(vi) {
			errs = errors.Join(errs, errors.New("invalid issuer public key: "+vi))
		}
	}

	if nn.options.OtelOptions.MetricsEnabled {
		if nn.options.OtelOptions.MetricsPort <= 0 || nn.options.OtelOptions.MetricsPort > 65535 {
			errs = errors.Join(errs, errors.New("invalid metrics port"))
		}
		if nn.options.OtelOptions.MetricsExporter == "" || !slices.Contains([]string{"file", "prometheus"}, nn.options.OtelOptions.MetricsExporter) {
			errs = errors.Join(errs, errors.New("invalid metrics exporter"))
		}
	}

	if nn.options.OtelOptions.TracesEnabled {
		if nn.options.OtelOptions.TracesExporter == "" || !slices.Contains([]string{"file", "http", "grpc"}, nn.options.OtelOptions.TracesExporter) {
			errs = errors.Join(errs, errors.New("invalid traces exporter"))
		}
		if nn.options.OtelOptions.TracesExporter == "http" || nn.options.OtelOptions.TracesExporter == "grpc" {
			if _, err := url.Parse(nn.options.OtelOptions.ExporterEndpoint); err != nil {
				errs = errors.Join(errs, errors.New("invalid traces exporter endpoint"))
			}
		}
	}

	return errs
}

func (nn *nexNode) Start() error {
	return nn.initializeSupervisionTree()
}

func (nn *nexNode) initializeSupervisionTree() error {
	var options gen.NodeOptions

	nodeName := "nex@localhost" // FIXME-- make configurable

	nodeID, err := nn.publicKey.PublicKey()
	if err != nil {
		fmt.Printf("Unable to start node '%s': %s\n", nodeName, err)
		return err
	}

	// create applications that must be started
	apps := []gen.ApplicationBehavior{
		observer.CreateApp(observer.Options{}),           // TODO: opt out of this via config
		actors.CreateNodeApp(nodeID, nn.nc, *nn.options), // copy options
	}
	options.Applications = apps

	// disable default logger to get rid of multiple logging to the os.Stdout
	options.Log.DefaultLogger.Disable = true

	// https://docs.ergo.services/basics/logging#process-logger
	// https://docs.ergo.services/tools/observer#log-process-page

	// starting node
	node, err := ergo.StartNode(gen.Atom(nodeName), options)
	if err != nil {
		fmt.Printf("Unable to start node '%s': %s\n", nodeName, err)
		return err
	}

	logger, err := node.Spawn(actors.CreateNodeLogger(nn.ctx, nn.options.Logger), gen.ProcessOptions{})
	if err != nil {
		return err
	}

	// NOTE: the supervised processes won't log their startup (Init) calls because the
	// logger won't have been in place. However, they will log stuff afterward
	err = node.LoggerAddPID(logger, "nexlogger")
	if err != nil {
		node.Log().Error("Failed to add logger", slog.String("error", err.Error()))
		return err
	}

	node.Log().Info("Nex node started")
	node.Log().Info("Observer Application started and available at http://localhost:9911")

	node.Wait()
	return nil
}
