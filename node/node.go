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
	"strings"
	"syscall"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/nex/node/actors"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/log"
)

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

	_, err := nn.publicKey.PublicKey()
	if err != nil {
		errs = errors.Join(errs, errors.New("could not produce a public key for this node. This should never happen"))
	}

	return errs
}

func (nn *nexNode) Start() error {
	return nn.initializeSupervisionTree()
}

func (nn *nexNode) initializeSupervisionTree() error {
	ctx := context.Background()

	actorSystem, _ := goakt.NewActorSystem("nexnode",
		goakt.WithLogger(log.DefaultLogger), // TODO: use "our" logger here. I think we just need a wrapper that conforms to this interface?
		goakt.WithPassivationDisabled(),
		// In the non-v2 version of goakt, these functions were supported.
		// TODO: figure out why they're gone or how we can plug in our own impls
		//goakt.WithTelemetry(telemetry),
		//goakt.WithTracing(),
		goakt.WithActorInitMaxRetries(3))

	// start the actor system
	_ = actorSystem.Start(ctx)

	// start the root actors
	agentSuper, err := actorSystem.Spawn(ctx, actors.AgentSupervisorActorName, actors.CreateAgentSupervisor(actorSystem, *nn.options))
	if err != nil {
		return err
	}
	inats := actors.CreateInternalNatsServer(*nn.options)

	_, err = actorSystem.Spawn(ctx, actors.InternalNatsServerActorName, inats)
	if err != nil {
		return err
	}
	allCreds := inats.CredentialsMap()

	_, err = actorSystem.Spawn(ctx, actors.HostServicesActorName, actors.CreateHostServices(nn.options.HostServiceOptions))
	if err != nil {
		return err
	}

	if !nn.options.DisableDirectStart {
		_, err = agentSuper.SpawnChild(ctx, actors.DirectStartActorName, actors.CreateDirectStartAgent(*nn.options))
		if err != nil {
			return err
		}
	}
	for _, agent := range nn.options.AgentOptions {
		// This map lookup works because the agent name is identical to the workload type
		_, err := agentSuper.SpawnChild(ctx, agent.Name, actors.CreateExternalAgent(allCreds[agent.Name], agent))
		if err != nil {
			return err
		}
	}

	pk, err := nn.publicKey.PublicKey()
	if err != nil {
		return err
	}
	_, err = actorSystem.Spawn(ctx, actors.ControlAPIActorName, actors.CreateControlAPI(nn.nc, pk))
	if err != nil {
		return err
	}

	running := make([]string, 0)
	for _, actor := range actorSystem.Actors() {
		running = append(running, actor.Name())
	}
	actorSystem.Logger().Infof("Actors started: %s", strings.Join(running, ","))

	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	// stop the actor system
	_ = actorSystem.Stop(ctx)
	os.Exit(0)

	return nil
}
