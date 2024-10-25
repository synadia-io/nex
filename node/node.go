package node

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"slices"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"

	"github.com/synadia-io/nex/internal/logger"
	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/nex/node/actors"
	goakt "github.com/tochemey/goakt/v2/actors"
)

type Node interface {
	Validate() error
	Start() error
}

type nexNode struct {
	ctx context.Context
	nc  *nats.Conn

	interrupt chan os.Signal
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

// Start is blocking and will not return until the node is stopped
// Can be stopped by canceling the provided context
func (nn *nexNode) Start() error {
	var cancel context.CancelFunc
	nn.ctx, cancel = context.WithCancel(nn.ctx)
	defer cancel()

	nn.interrupt = make(chan os.Signal, 1)
	go func() {
		<-nn.interrupt
		cancel()
	}()

	as, err := nn.initializeSupervisionTree()
	if err != nil {
		return err
	}

	<-nn.ctx.Done()
	nn.options.Logger.Info("Shutting down nexnode")
	return as.Stop(nn.ctx)
}

func (nn *nexNode) initializeSupervisionTree() (goakt.ActorSystem, error) {

	actorSystem, err := goakt.NewActorSystem("nexnode",
		goakt.WithLogger(logger.NewSlog(nn.options.Logger.Handler().WithGroup("system"))),
		goakt.WithPassivationDisabled(),
		// In the non-v2 version of goakt, these functions were supported.
		// TODO: figure out why they're gone or how we can plug in our own impls
		//goakt.WithTelemetry(telemetry),
		//goakt.WithTracing(),
		goakt.WithActorInitMaxRetries(3))
	if err != nil {
		return nil, err
	}

	// start the actor system
	err = actorSystem.Start(nn.ctx)
	if err != nil {
		return nil, err
	}

	// start the root actors
	agentSuper, err := actorSystem.Spawn(nn.ctx, actors.AgentSupervisorActorName, actors.CreateAgentSupervisor(actorSystem, *nn.options))
	if err != nil {
		return nil, err
	}
	inats := actors.CreateInternalNatsServer(*nn.options, nn.interrupt)

	_, err = actorSystem.Spawn(nn.ctx, actors.InternalNatsServerActorName, inats)
	if err != nil {
		return nil, err
	}
	allCreds := inats.CredentialsMap()

	_, err = actorSystem.Spawn(nn.ctx, actors.HostServicesActorName, actors.CreateHostServices(nn.options.HostServiceOptions))
	if err != nil {
		return nil, err
	}

	if !nn.options.DisableDirectStart {
		_, err = agentSuper.SpawnChild(nn.ctx, actors.DirectStartActorName, actors.CreateDirectStartAgent(*nn.options))
		if err != nil {
			return nil, err
		}
	}
	for _, agent := range nn.options.AgentOptions {
		// This map lookup works because the agent name is identical to the workload type
		_, err := agentSuper.SpawnChild(nn.ctx, agent.Name, actors.CreateExternalAgent(allCreds[agent.Name], agent))
		if err != nil {
			return nil, err
		}
	}

	pk, err := nn.publicKey.PublicKey()
	if err != nil {
		return nil, err
	}
	_, err = actorSystem.Spawn(nn.ctx, actors.ControlAPIActorName, actors.CreateControlAPI(nn.nc, pk))
	if err != nil {
		return nil, err
	}

	running := make([]string, len(actorSystem.Actors()))
	for i, actor := range actorSystem.Actors() {
		running[i] = actor.Name()
	}
	nn.options.Logger.Info("Actors started", slog.Any("running", running))

	return actorSystem, nil
}
