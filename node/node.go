package node

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	goakt "github.com/tochemey/goakt/v2/actors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/synadia-io/nex/internal/logger"
	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/nex/node/actors"
	actorproto "github.com/synadia-io/nex/node/actors/pb"
)

const (
	TagOS       = "nex.os"
	TagArch     = "nex.arch"
	TagCPUs     = "nex.cpucount"
	TagLameDuck = "nex.lameduck"
	TagNexus    = "nex.nexus"

	VERSION = "0.0.0"
)

var ReservedTagPrefixes = []string{"nex."}

type Node interface {
	Validate() error
	Start() error
}

type nexNode struct {
	ctx context.Context
	nc  *nats.Conn

	options       *models.NodeOptions
	publicKey     nkeys.KeyPair
	startedAt     time.Time
	runningActors map[string]*goakt.PID
}

func NewNexNode(serverKey nkeys.KeyPair, nc *nats.Conn, opts ...models.NodeOption) (Node, error) {
	if nc == nil {
		return nil, fmt.Errorf("no nats connection provided")
	}

	nn := &nexNode{
		ctx:           context.Background(),
		nc:            nc,
		publicKey:     serverKey,
		runningActors: make(map[string]*goakt.PID),

		options: &models.NodeOptions{
			Logger:                slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})),
			AgentHandshakeTimeout: 5000,
			ResourceDirectory:     "./resources",
			Tags: map[string]string{
				TagOS:       runtime.GOOS,
				TagArch:     runtime.GOARCH,
				TagCPUs:     fmt.Sprintf("%d", runtime.GOMAXPROCS(0)),
				TagLameDuck: "false",
				TagNexus:    "nexus",
			},
			ValidIssuers: []string{},
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

	for tag, _ := range nn.options.Tags {
		if slices.ContainsFunc(ReservedTagPrefixes,
			func(s string) bool {
				return strings.HasPrefix(tag, s)
			}) {
			errs = errors.Join(errs, errors.New("tag ["+tag+"] is using a reserved tag prefix"))
		}
	}

	return errs
}

// Start is blocking and will not return until the node is stopped
// Can be stopped by canceling the provided context
func (nn *nexNode) Start() error {
	var cancel context.CancelFunc
	nn.ctx, cancel = context.WithCancel(nn.ctx)
	defer cancel()

	interrupt := make(chan os.Signal, 1)
	signalReset(interrupt)
	go func() {
		<-interrupt
		cancel()
	}()

	as, err := nn.initializeSupervisionTree()
	if err != nil {
		return err
	}

	nn.startedAt = time.Now()
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
	nn.runningActors[actors.AgentSupervisorActorName] = agentSuper

	inats := actors.CreateInternalNatsServer(*nn.options)
	inatsActor, err := actorSystem.Spawn(nn.ctx, actors.InternalNatsServerActorName, inats)
	if err != nil {
		return nil, err
	}
	nn.runningActors[actors.InternalNatsServerActorName] = inatsActor

	allCreds := inats.CredentialsMap()

	hostServicesActor, err := actorSystem.Spawn(nn.ctx, actors.HostServicesActorName, actors.CreateHostServices(nn.options.HostServiceOptions))
	if err != nil {
		return nil, err
	}
	nn.runningActors[actors.HostServicesActorName] = hostServicesActor

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

	controlApiActor, err := actorSystem.Spawn(nn.ctx, actors.ControlAPIActorName,
		actors.CreateControlAPI(nn.nc, nn.options.Logger, pk, nn.auctionResponse))
	if err != nil {
		return nil, err
	}
	nn.runningActors[actors.ControlAPIActorName] = controlApiActor

	running := make([]string, len(actorSystem.Actors()))
	for i, actor := range actorSystem.Actors() {
		running[i] = actor.Name()
	}
	nn.options.Logger.Debug("Actors started", slog.Any("running", running))

	return actorSystem, nil
}

func (nn nexNode) auctionResponse(os, arch string, agentType []string, tags map[string]string) (*actorproto.AuctionResponse, error) {
	if os != runtime.GOOS || arch != runtime.GOARCH {
		return nil, errors.New("node did not satisfy auction os/arch requirements")
	}

	st := timestamppb.New(nn.startedAt)
	pk, err := nn.publicKey.PublicKey()
	if err != nil {
		nn.options.Logger.Error("Failed to get public key", slog.Any("err", err))
		return nil, err
	}

	resp := &actorproto.AuctionResponse{
		NodeId:    pk,
		Version:   VERSION,
		StartedAt: st,
		Tags:      nn.options.Tags,
	}

	agentSuper := nn.runningActors[actors.AgentSupervisorActorName]
	for _, c := range agentSuper.Children() {
		agentResp, err := agentSuper.Ask(nn.ctx, c, &actorproto.PingAgent{})
		if err != nil {
			nn.options.Logger.Error("Failed to ping agent", slog.Any("err", err))
			return nil, errors.New("failed to ping agent")
		}
		aR, ok := agentResp.(*actorproto.PingAgentResponse)
		if !ok {
			nn.options.Logger.Error("Failed to convert agent response")
			return nil, errors.New("failed to convert agent response")
		}
		resp.Status[c.Name()] = int32(len(aR.RunningWorkloads))
	}

	// Node must satisfy all agent types in auction request
	for name, _ := range resp.Status {
		if !slices.Contains(agentType, name) {
			nn.options.Logger.Error("node did not satisfy auction agent type requirements")
			return nil, errors.New("node did not satisfy auction agent type requirements")
		}
	}

	// Node must satisfy all tags in auction request
	for tag, value := range tags {
		if tV, ok := nn.options.Tags[tag]; !ok || tV != value {
			nn.options.Logger.Error("node did not satisfy auction tag requirements")
			return nil, errors.New("node did not satisfy auction tag requirements")
		}
	}

	return resp, nil
}
