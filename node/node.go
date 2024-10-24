package node

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"ergo.services/application/observer"
	"ergo.services/ergo"
	"ergo.services/ergo/gen"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/nex/node/actors"
)

const ergoNodeName = "nex@localhost"

type Node interface {
	Validate() error
	Start() error
}

type nexNode struct {
	ctx context.Context
	nc  *nats.Conn

	options   *models.NodeOptions
	publicKey nkeys.KeyPair
}

func NewNexNode(serverKey nkeys.KeyPair, nc *nats.Conn, opts ...models.NodeOption) (Node, error) {
	if nc == nil {
		return nil, fmt.Errorf("no nats connection provided")
	}

	nn := &nexNode{
		ctx:       context.Background(),
		nc:        nc,
		publicKey: serverKey,

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
			DisableDirectStart: false,
			AgentOptions:       []models.AgentOptions{},
			HostServiceOptions: models.HostServiceOptions{
				Services: make(map[string]models.ServiceConfig),
			},
			Observer: models.ObserverOptions{
				Enabled: false,
				Host:    "127.0.0.1",
				Port:    9911,
			},
		},
	}

	for _, opt := range opts {
		if opt != nil {
			opt(nn.options)
		}
	}

	err := nn.Validate()
	if err != nil {
		return nil, err
	}

	return nn, nil
}

func (nn nexNode) Validate() error {
	var errs error

	if nn.options.Logger == nil {
		errs = errors.Join(errs, errors.New("logger is nil"))
	}

	if nn.options.AgentHandshakeTimeout <= 0 {
		errs = errors.Join(errs, errors.New("agent handshake timeout must be greater than 0"))
	}

	if len(nn.options.AgentOptions) < 1 && nn.options.DisableDirectStart {
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

	nodeID, err := nn.publicKey.PublicKey()
	if err != nil {
		fmt.Printf("Unable to start node; %s\n", err)
		return err
	}

	// create applications that must be started
	options.Applications = []gen.ApplicationBehavior{
		actors.CreateNodeApp(nodeID, nn.nc, *nn.options), // copy options
	}

	if nn.options.Observer.Enabled {
		options.Applications = append(options.Applications,
			observer.CreateApp(observer.Options{
				Host: nn.options.Observer.Host,
				Port: nn.options.Observer.Port,
			},
			))
	}

	// disable default logger to get rid of multiple logging to the os.Stdout
	options.Log.DefaultLogger.Disable = true
	options.Log.Level = gen.LogLevelTrace // can stay at trace because slog.Log level will handle the output

	// DO NOT REMOVE: the ergo.services library will transmit metrics by default and can only be seen if TRACE level logs are enabled.
	// This disables that behavior
	// ref: https://github.com/ergo-services/ergo/blob/364c7ef006ed6eef2775c23dad48ab3a12b7085f/app/system/metrics.go#L102
	options.Env = make(map[gen.Env]any)
	options.Env["disable_metrics"] = "true"

	// starting node
	node, err := ergo.StartNode(gen.Atom(ergoNodeName), options)
	if err != nil {
		fmt.Printf("Unable to start node; %s\n", err)
		return err
	}

	// https://docs.ergo.services/basics/logging#process-logger
	// https://docs.ergo.services/tools/observer#log-process-page
	logger, err := node.Spawn(actors.CreateNodeLogger(nn.ctx, nn.options.Logger), gen.ProcessOptions{})
	if err != nil {
		return err
	}

	// NOTE: the supervised processes won't log their startup (Init) calls because the
	// logger won't have been in place. However, they will log stuff afterward
	err = node.LoggerAddPID(logger, "nexlogger", gen.DefaultLogLevels...)
	if err != nil {
		node.Log().Error("Failed to add logger", slog.String("error", err.Error()))
		return err
	}

	node.Log().Info("Nex node started")
	if nn.options.Observer.Enabled {
		node.Log().Info("Observer Application started", slog.String("server", fmt.Sprintf("http://%s:%d", nn.options.Observer.Host, nn.options.Observer.Port)))
	}

	nn.installSignalHandlers(node)
	node.Wait()

	return nil
}

func (nn *nexNode) installSignalHandlers(node gen.Node) {
	go func() {
		nn.options.Logger.Info("Installing signal handlers")

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

		sig := <-sigs
		if sig == nil {
			signal.Reset()
			return
		}

		signal.Reset()

		nn.options.Logger.Info("Stopping node")
		node.Stop()
	}()
}
