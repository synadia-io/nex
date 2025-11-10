package nex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/synadia-io/nex/internal"
	"github.com/synadia-io/nex/internal/aregistrar"
	"github.com/synadia-io/nex/internal/credentials"
	eventemitter "github.com/synadia-io/nex/internal/event_emitter"
	"github.com/synadia-io/nex/internal/idgen"
	secretstore "github.com/synadia-io/nex/internal/secret_store"
	"github.com/synadia-io/nex/internal/state"
	"github.com/synadia-io/nex/models"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nats.go/micro"
	"github.com/nats-io/nkeys"
	sdk "github.com/synadia-io/nex/sdk/go/agent"
)

type (
	NexNodeOption func(*NexNode) error
	NexNode       struct {
		ctx       context.Context
		cancel    context.CancelFunc
		id        string
		version   string
		commit    string
		builddate string

		logger    *slog.Logger
		startTime time.Time

		allowRemoteAgentRegistration bool
		auctionMap                   *internal.TTLMap

		nodeKeypair  nkeys.KeyPair
		nodeXKeypair nkeys.KeyPair

		signingKey string
		issuerAcct string

		name      string
		nexus     string
		tags      map[string]string
		nodeState models.NodeState

		agentRestartLimit int
		// Embedded agents
		embeddedRunners []*sdk.Runner
		// Config based agents
		localRunners []*internal.AgentProcess
		agentWatcher *internal.AgentWatcher

		// List of active agents
		registeredAgents *internal.AgentRegistrations

		minter       models.CredVendor
		state        models.NexNodeState
		auctioneer   models.Auctioneer
		idgen        models.IDGen
		aregistrar   models.AgentRegistrar
		secretStore  models.SecretStore
		eventEmitter models.EventEmitter

		nc          *nats.Conn
		service     micro.Service
		jsCtx       jetstream.JetStream
		server      *server.Server
		serverCreds *models.NatsConnectionData

		nodeShutdown          chan struct{}
		shutdownMu            sync.RWMutex
		shutdownDueToLameduck bool
	}
)

const (
	defaultNexNodeName           = "nexnode"
	defaultNexNodeNexus          = "nexus"
	defaultAuctionTTLMapDuration = time.Second * 10
	defaultAgentWatcherRestarts  = 3
)

func NewNexNode(opts ...NexNodeOption) (*NexNode, error) {
	kp, err := nkeys.CreateServer()
	if err != nil {
		return nil, err
	}

	xkp, err := nkeys.CreateCurveKeys()
	if err != nil {
		return nil, err
	}

	n := &NexNode{
		ctx:       context.Background(),
		version:   "0.0.0",
		commit:    "development",
		builddate: "unknown",

		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		startTime: time.Time{},

		name:  defaultNexNodeName,
		nexus: defaultNexNodeNexus,
		tags: map[string]string{
			models.TagOS:       runtime.GOOS,
			models.TagArch:     runtime.GOARCH,
			models.TagCPUs:     strconv.Itoa(runtime.GOMAXPROCS(0)),
			models.TagLameDuck: "false",
		},
		nodeState: models.NodeStateStarting,

		agentRestartLimit: defaultAgentWatcherRestarts,
		embeddedRunners:   make([]*sdk.Runner, 0),
		localRunners:      make([]*internal.AgentProcess, 0),

		minter:       &credentials.FullAccessMinter{NatsServers: []string{nats.DefaultURL}},
		state:        &state.NoState{},
		auctioneer:   nil,
		idgen:        idgen.NewNuidGen(),
		aregistrar:   &aregistrar.AllowAllRegistrar{},
		secretStore:  &secretstore.NoStore{},
		eventEmitter: &eventemitter.NoEmit{},

		nodeKeypair:                  kp,
		nodeXKeypair:                 xkp,
		allowRemoteAgentRegistration: false,
		auctionMap:                   internal.NewTTLMap(defaultAuctionTTLMapDuration),

		nc:     nil,
		server: nil,

		nodeShutdown: make(chan struct{}, 1),
	}

	var errs error
	for _, opt := range opts {
		if opt != nil {
			errs = errors.Join(errs, opt(n))
		}
	}
	if errs != nil {
		return nil, errs
	}

	n.id, err = n.nodeKeypair.PublicKey()
	if err != nil {
		return nil, err
	}

	n.ctx, n.cancel = context.WithCancel(n.ctx)
	n.tags[models.TagNexus] = n.nexus
	n.tags[models.TagNodeName] = n.name

	pubKey, err := n.nodeKeypair.PublicKey()
	if err != nil {
		return nil, err
	}
	n.registeredAgents = internal.NewAgentRegistrations(n.ctx, pubKey, n.nc, n.logger.WithGroup("agent-registrations"))

	var agentStarter sync.WaitGroup
	agentStarter.Add(len(n.embeddedRunners) + len(n.localRunners))
	n.agentWatcher = internal.NewAgentWatcher(n.ctx, n.nc, n.nodeKeypair, n.logger.WithGroup("agent-watcher"), n.eventEmitter, n.agentRestartLimit, &agentStarter)

	return n, nil
}

func (n *NexNode) Start() error {
	version, ok := n.ctx.Value("VERSION").(string)
	if ok {
		n.version = version
	}
	commit, ok := n.ctx.Value("COMMIT").(string)
	if ok {
		n.commit = commit
	}
	builddate, ok := n.ctx.Value("BUILDDATE").(string)
	if ok {
		n.builddate = builddate
	}

	if n.server != nil {
		n.server.ConfigureLogger()
		n.server.Start()
		n.logger.Info("Using internal nats server", slog.String("url", n.server.ClientURL()))

		startNats := time.Now()
		for !n.server.Running() {
			if time.Since(startNats) > 5*time.Second {
				return errors.New("internal nats server failed to start")
			}
			time.Sleep(250 * time.Millisecond)
		}
	}

	var err error
	if n.nc == nil && n.server != nil {
		n.nc, err = configureNatsConnection(n.serverCreds)
		if err != nil {
			return err
		}
	} else if n.nc == nil {
		return errors.New("nats connection is not set; please provide a valid nats connection or an internal nats server")
	}

	n.startTime = time.Now()
	n.logger.Info("Starting nex node",
		slog.String("version", n.version),
		slog.String("commit", n.commit),
		slog.String("build_date", n.builddate),
		slog.String("node_id", n.id),
		slog.String("name", n.name),
		slog.String("nexus", n.nexus),
		slog.String("nats_server", n.nc.ConnectedUrl()),
		slog.String("start_time", n.startTime.Format(time.RFC3339)))

	n.jsCtx, err = jetstream.New(n.nc)
	if err != nil {
		return err
	}

	n.service, err = micro.AddService(n.nc, micro.Config{
		Name:        "nexnode",
		Version:     n.version,
		Description: fmt.Sprintf("Commit: %s | Build date: %s", n.commit, n.builddate),
	})
	if err != nil {
		return err
	}

	var errs error
	// System only endpoints
	errs = errors.Join(errs, n.service.AddEndpoint("PingNexus", micro.HandlerFunc(n.handlePing()), micro.WithEndpointSubject(models.PingSubscribeSubject()), micro.WithEndpointQueueGroup(n.id)))
	errs = errors.Join(errs, n.service.AddEndpoint("PingNode", micro.HandlerFunc(n.handlePing()), micro.WithEndpointSubject(models.DirectPingSubscribeSubject(n.id)), micro.WithEndpointQueueGroup(n.id)))
	errs = errors.Join(errs, n.service.AddEndpoint("GetNodeInfo", micro.HandlerFunc(n.handleNodeInfo()), micro.WithEndpointSubject(models.NodeInfoSubscribeSubject(n.id)), micro.WithEndpointQueueGroup(n.id)))
	errs = errors.Join(errs, n.service.AddEndpoint("SetLameduck", micro.HandlerFunc(n.handleLameduck()), micro.WithEndpointSubject(models.LameduckSubscribeSubject(n.id)), micro.WithEndpointQueueGroup(n.id)))
	errs = errors.Join(errs, n.service.AddEndpoint("GetAgentIdByName", micro.HandlerFunc(n.handleGetAgentIDByName()), micro.WithEndpointSubject(models.GetAgentIdByNameSubject(n.id)), micro.WithEndpointQueueGroup(n.id)))
	// System only agent endpoints
	if n.allowRemoteAgentRegistration {
		n.logger.Warn("remote registration enabled. agents can remotely register to this node")
		errs = errors.Join(errs, n.service.AddEndpoint("RegisterRemoteAgent", micro.HandlerFunc(n.handleRegisterRemoteAgent()), micro.WithEndpointSubject(models.AgentAPIInitRemoteRegisterSubscribeSubject(n.nexus)), micro.WithEndpointQueueGroup(n.nexus)))
	}
	errs = errors.Join(errs, n.service.AddEndpoint("RegisterAgent", micro.HandlerFunc(n.handleRegisterAgent()), micro.WithEndpointSubject(models.AgentAPIRegisterSubscribeSubject(n.id)), micro.WithEndpointQueueGroup(n.id)))
	// User endpoints
	errs = errors.Join(errs, n.service.AddEndpoint("AuctionRequest", micro.HandlerFunc(n.handleAuction()), micro.WithEndpointSubject(models.AuctionSubscribeSubject()), micro.WithEndpointQueueGroup(n.id)))
	errs = errors.Join(errs, n.service.AddEndpoint("StopWorkload", micro.HandlerFunc(n.handleStopWorkload()), micro.WithEndpointSubject(models.UndeploySubscribeSubject()), micro.WithEndpointQueueGroup(n.id)))
	errs = errors.Join(errs, n.service.AddEndpoint("AuctionDeployWorkload", micro.HandlerFunc(n.handleAuctionDeployWorkload()), micro.WithEndpointSubject(models.AuctionDeploySubscribeSubject()), micro.WithEndpointQueueGroup(n.id)))
	errs = errors.Join(errs, n.service.AddEndpoint("CloneWorkload", micro.HandlerFunc(n.handleCloneWorkload()), micro.WithEndpointSubject(models.CloneWorkloadSubscribeSubject()), micro.WithEndpointQueueGroup(n.id)))
	errs = errors.Join(errs, n.service.AddEndpoint("NamespacePingRequest", micro.HandlerFunc(n.handleNamespacePing()), micro.WithEndpointSubject(models.NamespacePingSubscribeSubject()), micro.WithEndpointQueueGroup(n.id)))

	if errs != nil {
		return errs
	}

	err = n.eventEmitter.EmitEvent(n.id, models.NexNodeStartedEvent{
		Id:    n.id,
		Name:  n.name,
		Nexus: n.nexus,
		Tags:  n.tags,
		Type:  "io.synadia.nex.event.nexnode_started",
	})
	if err != nil {
		n.logger.Error("failed to emit nex node started event", slog.String("err", err.Error()))
	}
	go n.heartbeat()

	for _, e := range n.service.Info().Endpoints {
		if e.QueueGroup != micro.DefaultQueueGroup {
			n.logger.Debug("Subscribed to nats subject", slog.String("subject", e.Subject), slog.String("queue_group", e.QueueGroup))
		} else {
			n.logger.Debug("Subscribed to nats subject", slog.String("subject", e.Subject))
		}
	}

	// Start agents via constructor
	for _, runner := range n.embeddedRunners {
		id := n.idgen.Generate(nil)
		connData, err := n.minter.MintRegister(id, n.id)
		if err != nil {
			n.logger.Error("failed to mint register", slog.String("err", err.Error()))
			continue
		}
		go n.agentWatcher.StartEmbeddedAgent(id, runner, connData)
	}

	// start local agents
	for _, agentProcess := range n.localRunners {
		agentProcess.HostNode = n.id
		agentProcess.ID = n.idgen.Generate(nil)
		connData, err := n.minter.MintRegister(agentProcess.ID, n.id)
		if err != nil {
			n.logger.Error("failed to mint register", slog.String("err", err.Error()))
			continue
		}
		go n.agentWatcher.StartLocalBinaryAgent(agentProcess, connData)
	}

	n.agentWatcher.WaitForAgents()
	if n.registeredAgents.Count() == 0 {
		n.logger.Warn("nex node started without any agents")
	}

	n.logger.Info("nex node ready")
	n.nodeState = models.NodeStateRunning
	return nil
}

func (n *NexNode) IsReady() bool {
	return n.nodeState == models.NodeStateRunning
}

func (n *NexNode) Shutdown() error {
	if n.nodeState == models.NodeStateStopping {
		n.logger.Warn("nex node already shutting down")
		return nil
	}

	if n.nodeState == models.NodeStateLameduck {
		n.shutdownMu.Lock()
		n.shutdownDueToLameduck = true
		n.shutdownMu.Unlock()
	}
	n.nodeState = models.NodeStateStopping

	n.agentWatcher.Shutdown()

	err := n.service.Stop()
	if err != nil {
		n.logger.Error("failed to stop micro service", slog.String("err", err.Error()))
	}

	pubKey, err := n.nodeKeypair.PublicKey()
	if err != nil {
		n.logger.Error("failed to get node public key", slog.String("err", err.Error()))
	}

	err = n.eventEmitter.EmitEvent(n.id, models.NexNodeStoppedEvent{
		Id: pubKey,
	})
	if err != nil {
		n.logger.Error("failed to emit nex node stopped event", slog.String("err", err.Error()))
	}

	if n.nc != nil && !n.nc.IsClosed() {
		err = n.nc.Drain()
		if err != nil {
			n.logger.Error("failed to drain nats connection", slog.String("err", err.Error()))
		}
	}

	if n.server != nil {
		n.logger.Debug("stopping internal nats server", slog.String("url", n.server.ClientURL()))
		n.server.Shutdown()
	}

	n.logger.Info("nex node stopped", slog.String("uptime", time.Since(n.startTime).String()))

	// Non-blocking send to avoid hanging if channel is already full
	select {
	case n.nodeShutdown <- struct{}{}:
	default:
		// Channel already has a shutdown signal, ignore
	}
	return nil
}

func (n *NexNode) WaitForShutdown() error {
	for {
		select {
		case <-n.ctx.Done(): // shutdown by context
			n.logger.Warn("shutdown by context cancellation")
			return n.Shutdown()
		case <-n.nodeShutdown: // shutdown by command, recommended
			n.shutdownMu.RLock()
			wasLameduck := n.shutdownDueToLameduck
			n.shutdownMu.RUnlock()
			if wasLameduck {
				return models.ErrLameduckShutdown
			}
			return nil
		}
	}
}

func (n *NexNode) enterLameduck(delay time.Duration) {
	n.nodeState = models.NodeStateLameduck

	go func() {
		time.Sleep(delay)
		err := n.Shutdown()
		if err != nil {
			n.logger.Error("failed to shutdown nex node", slog.String("err", err.Error()))
		}
	}()

	pubKey, err := n.nodeKeypair.PublicKey()
	if err != nil {
		n.logger.Error("failed to get node public key", slog.String("err", err.Error()))
	}
	err = n.eventEmitter.EmitEvent(n.id, models.NexNodeLameduckSetEvent{
		Id:   pubKey,
		Time: time.Now().Add(delay),
	})
	if err != nil {
		n.logger.Error("failed to emit nex node lameduck event", slog.String("err", err.Error()))
	}
}

func configureNatsConnection(connData *models.NatsConnectionData) (*nats.Conn, error) {
	if connData.ConnName == "" {
		connData.ConnName = "nexnode"
	}

	opts := []nats.Option{
		nats.Name(connData.ConnName),
		nats.MaxReconnects(-1),
		nats.Timeout(10 * time.Second),
	}

	if connData.TlsCert != "" && connData.TlsKey != "" {
		opts = append(opts, nats.ClientCert(connData.TlsCert, connData.TlsKey))
	}
	if connData.TlsCa != "" {
		opts = append(opts, nats.RootCAs(connData.TlsCa))
	}
	if connData.TlsFirst {
		opts = append(opts, nats.TLSHandshakeFirst())
	}

	switch {
	case connData.NatsUserSeed != "" && connData.NatsUserJwt != "": // Use seed + jwt
		opts = append(opts, nats.UserJWTAndSeed(connData.NatsUserJwt, connData.NatsUserSeed))
	case connData.NatsUserNkey != "" && connData.NatsUserSeed != "": // User nkey
		opts = append(opts, nats.Nkey(connData.NatsUserNkey, func(nonce []byte) ([]byte, error) {
			kp, err := nkeys.FromSeed([]byte(connData.NatsUserSeed))
			if err != nil {
				return nil, err
			}
			return kp.Sign(nonce)
		}))
	case connData.NatsUserName != "" && connData.NatsUserPassword != "": // Use user + password
		opts = append(opts, nats.UserInfo(connData.NatsUserName, connData.NatsUserPassword))
	}

	if len(connData.NatsServers) == 0 {
		connData.NatsServers = []string{nats.DefaultURL}
	}

	nc, err := nats.Connect(strings.Join(connData.NatsServers, ","), opts...)
	if err != nil {
		return nil, err
	}

	return nc, nil
}
