package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"runtime"
	"slices"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/nats-io/nuid"
	"github.com/splode/fname"
	goakt "github.com/tochemey/goakt/v2/actors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/synadia-io/nex/internal/logger"
	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/nex/node/internal/actors"
	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
)

const (
	VERSION = "0.0.0"
)

type Node interface {
	Validate() error
	Start() error
}

type nexNode struct {
	ctx       context.Context
	nc        *nats.Conn
	interrupt chan os.Signal

	options     *models.NodeOptions
	publicKey   nkeys.KeyPair
	startedAt   time.Time
	actorSystem goakt.ActorSystem

	auctionMap *TTLMap
}

func NewNexNode(serverKey nkeys.KeyPair, nc *nats.Conn, opts ...models.NodeOption) (Node, error) {
	if nc == nil {
		return nil, fmt.Errorf("no nats connection provided")
	}

	rng := fname.NewGenerator()
	nodeName, err := rng.Generate()
	if err != nil {
		nodeName = "nexnode"
	}

	xkey, err := nkeys.CreateCurveKeys()
	if err != nil {
		return nil, err
	}

	nn := &nexNode{
		ctx:        context.Background(),
		nc:         nc,
		publicKey:  serverKey,
		auctionMap: NewTTLMap(time.Second * 10),

		options: &models.NodeOptions{
			Logger:                slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})),
			AgentHandshakeTimeout: 5000,
			ResourceDirectory:     "./resources",
			Xkey:                  xkey,
			Tags: map[string]string{
				models.TagOS:       runtime.GOOS,
				models.TagArch:     runtime.GOARCH,
				models.TagCPUs:     fmt.Sprintf("%d", runtime.GOMAXPROCS(0)),
				models.TagLameDuck: "false",
				models.TagNexus:    "nexus",
				models.TagNodeName: nodeName,
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

	if nn.options.Errs != nil {
		return nil, nn.options.Errs
	}

	err = nn.Validate()
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

	err = nkeys.CompatibleKeyPair(nn.options.Xkey, nkeys.PrefixByteCurve)
	if err != nil {
		errs = errors.Join(errs, errors.New("node xkeypair is not a curve keypair"))
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
	signalReset(nn.interrupt)
	go func() {
		<-nn.interrupt
		cancel()
	}()

	nn.startedAt = time.Now()
	err := nn.initializeSupervisionTree()
	if err != nil {
		return err
	}

	<-nn.ctx.Done()
	nn.options.Logger.Info("Shutting down nexnode")
	return nn.actorSystem.Stop(nn.ctx)
}

func (nn *nexNode) initializeSupervisionTree() error {
	var err error
	restartDirective := goakt.NewRestartDirective()
	restartDirective.WithLimit(3, 30*time.Second)

	nn.actorSystem, err = goakt.NewActorSystem("nexnode",
		goakt.WithLogger(logger.NewSlog(nn.options.Logger.Handler().WithGroup("system"))),
		goakt.WithPassivationDisabled(),
		// In the non-v2 version of goakt, these functions were supported.
		// TODO: figure out why they're gone or how we can plug in our own impls
		//goakt.WithTelemetry(telemetry),
		//goakt.WithTracing(),
		goakt.WithSupervisorDirective(restartDirective),
		goakt.WithActorInitMaxRetries(3))
	if err != nil {
		return err
	}

	// start the actor system
	err = nn.actorSystem.Start(nn.ctx)
	if err != nil {
		return err
	}

	// start the root actors
	agentSuper, err := nn.actorSystem.Spawn(nn.ctx, actors.AgentSupervisorActorName, actors.CreateAgentSupervisor(nn.actorSystem, *nn.options))
	if err != nil {
		return err
	}

	inats := actors.CreateInternalNatsServer(*nn.options)
	_, err = nn.actorSystem.Spawn(nn.ctx, actors.InternalNatsServerActorName, inats)
	if err != nil {
		return err
	}

	allCreds := inats.CredentialsMap()

	_, err = nn.actorSystem.Spawn(nn.ctx, actors.HostServicesActorName, actors.CreateHostServices(nn.options.HostServiceOptions))
	if err != nil {
		return err
	}

	pk, err := nn.publicKey.PublicKey()
	if err != nil {
		return err
	}

	if !nn.options.DisableDirectStart {
		_, err = agentSuper.SpawnChild(nn.ctx, actors.DirectStartActorName, actors.CreateDirectStartAgent(nn.nc, pk, *nn.options, nn.options.Logger.WithGroup("direct_start")))
		if err != nil {
			return err
		}
	}
	for _, agent := range nn.options.AgentOptions {
		// This map lookup works because the agent name is identical to the workload type
		_, err := agentSuper.SpawnChild(nn.ctx, agent.Name, actors.CreateExternalAgent(nn.options.Logger.WithGroup(agent.Name), allCreds[agent.Name], agent))
		if err != nil {
			return err
		}
	}

	_, err = nn.actorSystem.Spawn(nn.ctx, actors.ControlAPIActorName,
		actors.CreateControlAPI(nn.nc, nn.options.Logger, pk, nn))
	if err != nil {
		return err
	}

	running := make([]string, len(nn.actorSystem.Actors()))
	for i, actor := range nn.actorSystem.Actors() {
		running[i] = actor.Name()
	}
	nn.options.Logger.Debug("Actors started", slog.Any("running", running))

	return nil
}

func (nn *nexNode) Auction(auctionId string, agentType []string, tags map[string]string) (*actorproto.AuctionResponse, error) {
	if lameduck, ok := nn.options.Tags[models.TagLameDuck]; ok && lameduck == "true" {
		nn.options.Logger.Debug("node is in lame duck mode; not participating in auction")
		return nil, nil
	}

	xkp, err := nkeys.CreateCurveKeys()
	if err != nil {
		return nil, err
	}

	xkPub, err := xkp.PublicKey()
	if err != nil {
		return nil, err
	}

	// Gets new auction id & replace nodeid
	bidderId := nuid.New().Next()
	nn.auctionMap.Put(bidderId, auctionId, xkp)

	resp := &actorproto.AuctionResponse{
		BidderId:   bidderId,
		Version:    VERSION,
		TargetXkey: xkPub,
		StartedAt:  timestamppb.New(nn.startedAt),
		Tags:       nn.options.Tags,
		Status:     make(map[string]int32),
	}

	_, agentSuper, err := nn.actorSystem.ActorOf(nn.ctx, actors.AgentSupervisorActorName)
	if err != nil {
		nn.options.Logger.Error("Failed to get agent supervisor", slog.Any("err", err))
		return nil, err
	}
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
	for _, aT := range agentType {
		if _, ok := resp.Status[aT]; !ok {
			nn.options.Logger.Debug("node did not satisfy auction agent type requirements")
			return nil, nil
		}
	}

	// Node must satisfy all tags in auction request
	for tag, value := range tags {
		if tV, ok := nn.options.Tags[tag]; !ok || tV != value {
			nn.options.Logger.Debug("node did not satisfy auction tag requirements")
			return nil, nil
		}
	}

	return resp, nil
}

func (nn nexNode) Ping() (*actorproto.PingNodeResponse, error) {
	st := timestamppb.New(nn.startedAt)
	pk, err := nn.publicKey.PublicKey()
	if err != nil {
		nn.options.Logger.Error("Failed to get public key", slog.Any("err", err))
		return nil, err
	}

	nexus, ok := nn.options.Tags[models.TagNexus]
	if !ok {
		nexus = "_"
		nn.options.Logger.Warn("Nexus tag not found when responding to ping request")
	}

	resp := &actorproto.PingNodeResponse{
		NodeId:        pk,
		Nexus:         nexus,
		Version:       VERSION,
		StartedAt:     st,
		Tags:          nn.options.Tags,
		RunningAgents: make(map[string]int32),
	}

	_, agentSuper, err := nn.actorSystem.ActorOf(nn.ctx, actors.AgentSupervisorActorName)
	if err != nil {
		nn.options.Logger.Error("Failed to get agent supervisor", slog.Any("err", err))
		return nil, err
	}
	for _, c := range agentSuper.Children() {
		agentResp, err := agentSuper.Ask(nn.ctx, c, &actorproto.QueryWorkloads{})
		if err != nil {
			nn.options.Logger.Error("Failed to ping agent", slog.Any("err", err))
			return nil, errors.New("failed to ping agent")
		}
		aR, ok := agentResp.(*actorproto.WorkloadList)
		if !ok {
			nn.options.Logger.Error("Failed to convert agent response")
			return nil, errors.New("failed to convert agent response")
		}
		resp.RunningAgents[c.Name()] = int32(len(aR.Workloads))
	}

	return resp, nil
}

func (nn nexNode) GetInfo(namespace string) (*actorproto.NodeInfo, error) {
	pk, err := nn.publicKey.PublicKey()
	if err != nil {
		nn.options.Logger.Error("Failed to get public key", slog.Any("err", err))
		return nil, err
	}
	resp := &actorproto.NodeInfo{
		Id: pk,
		//FINDME
		//TargetXkey: nn.options.
		Tags:    nn.options.Tags,
		Uptime:  time.Since(nn.startedAt).String(),
		Version: VERSION,
	}

	_, agentSuper, err := nn.actorSystem.ActorOf(nn.ctx, actors.AgentSupervisorActorName)
	if err != nil {
		nn.options.Logger.Error("Failed to get agent supervisor", slog.Any("err", err))
		return nil, err
	}
	for _, c := range agentSuper.Children() {
		agentResp, err := agentSuper.Ask(nn.ctx, c, &actorproto.QueryWorkloads{})
		if err != nil {
			nn.options.Logger.Error("Failed to ping agent", slog.Any("err", err))
			return nil, errors.New("failed to ping agent")
		}
		wL, ok := agentResp.(*actorproto.WorkloadList)
		if !ok {
			nn.options.Logger.Error("Failed to convert agent response")
			return nil, errors.New("failed to convert agent response")
		}
		for _, w := range wL.Workloads {
			if namespace == "system" || w.Namespace == namespace {
				resp.Workloads = append(resp.Workloads, &actorproto.WorkloadSummary{
					Id:           w.Id,
					Name:         w.Name,
					Runtime:      w.Runtime,
					StartedAt:    timestamppb.New(w.StartedAt.AsTime()),
					WorkloadType: c.Name(),
					State:        w.State,
				})
			}
		}
	}

	return resp, nil
}

func (nn nexNode) SetLameDuck(ctx context.Context) {
	nn.options.Tags[models.TagLameDuck] = "true"

	go func() {
		<-ctx.Done()
		nn.interrupt <- os.Interrupt
	}()
}

func (nn nexNode) IsTargetNode(inId string) (bool, nkeys.KeyPair, error) {
	pub, err := nn.publicKey.PublicKey()
	if err != nil {
		return false, nil, err
	}
	if inId == pub {
		return true, nn.options.Xkey, nil
	}
	if nn.auctionMap.Exists(inId) {
		auctionId, kp := nn.auctionMap.Get(inId)
		nn.options.Logger.Debug("Accepting workload from auction", slog.String("auctionId", auctionId))
		nn.auctionMap.Delete(inId)
		return true, kp, nil
	}
	return false, nil, nil
}

func (nn nexNode) EncryptPayload(payload []byte, to string) ([]byte, string, error) {
	xPub, err := nn.options.Xkey.PublicKey()
	if err != nil {
		return nil, "", err
	}

	if to == "" {
		to = xPub
	}

	payloadEnc, err := nn.options.Xkey.Seal(payload, to)
	if err != nil {
		return nil, "", err
	}
	return payloadEnc, xPub, nil
}

func (nn nexNode) DecryptPayload(payload []byte) ([]byte, error) {
	xPub, err := nn.options.Xkey.PublicKey()
	if err != nil {
		return nil, err
	}
	return nn.options.Xkey.Open(payload, xPub)
}

func (nn nexNode) EmitEvent(inNamespace string, inEvent json.RawMessage) error {
	event, err := inEvent.MarshalJSON()
	if err != nil {
		return err
	}
	return nn.nc.Publish(models.EventAPIPrefix+"."+inNamespace, event)
}
