package node

import (
	"fmt"
	"log/slog"
	"os"

	"ergo.services/application/observer"
	"ergo.services/ergo"
	"ergo.services/ergo/gen"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/node/actors"
	"github.com/synadia-io/nex/node/options"
)

type Node interface {
	Validate() error
	Start()
}

type nexNode struct {
	nc *nats.Conn

	// NOTE on refactor: the node struct for an operational node (state) should be
	// separate from the options that started it, so the options can be passed around
	// freely
	options *options.NodeOptions
}

func NewNexNode(nc *nats.Conn, opts ...options.NodeOption) (Node, error) {
	if nc == nil {
		return nil, fmt.Errorf("no nats connection provided")
	}

	nn := &nexNode{
		nc: nc,
		options: &options.NodeOptions{
			Logger:                slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})),
			AgentHandshakeTimeout: 5000,
			ResourceDirectory:     "./resources",
			Tags:                  make(map[string]string),
			ValidIssuers:          []string{},
			OtelOptions: options.OTelOptions{
				MetricsEnabled:   false,
				MetricsPort:      8085,
				MetricsExporter:  "file",
				TracesEnabled:    false,
				TracesExporter:   "file",
				ExporterEndpoint: "127.0.0.1:14532",
			},
			WorkloadOptions: []options.WorkloadOptions{},
			HostServiceOptions: options.HostServiceOptions{
				Services: make(map[string]options.ServiceConfig),
			},
		},
	}

	for _, opt := range opts {
		opt(nn.options)
	}

	err := nn.options.Validate()
	if err != nil {
		return nil, err
	}

	return nn, nil
}

func (nn *nexNode) Validate() error {
	// already rejected nil nats connection, so no "node state"
	// validation needed.

	return nn.options.Validate()
}

func (nn *nexNode) Start() {
	nn.initializeSupervisionTree()
}

func (nn *nexNode) initializeSupervisionTree() {
	var options gen.NodeOptions

	// create applications that must be started
	apps := []gen.ApplicationBehavior{
		observer.CreateApp(observer.Options{}), // TODO: opt out of this via config
		actors.CreateNodeApp(*nn.options),      // copy options
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
