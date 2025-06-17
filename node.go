package nex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/synadia-labs/nex/internal"
	"github.com/synadia-labs/nex/internal/aregistrar"
	"github.com/synadia-labs/nex/internal/credentials"
	"github.com/synadia-labs/nex/internal/idgen"
	"github.com/synadia-labs/nex/internal/state"
	"github.com/synadia-labs/nex/models"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nats.go/micro"
	"github.com/nats-io/nkeys"
	sdk "github.com/synadia-io/nexlet.go/agent"
)

type (
	NexNodeOption func(*NexNode) error
	NexNode       struct {
		ctx       context.Context
		cancel    context.CancelFunc
		version   string
		commit    string
		builddate string

		logger    *slog.Logger
		startTime time.Time

		allowAgentRegistration bool
		auctionMap             *internal.TTLMap

		nodeKeypair  nkeys.KeyPair
		nodeXKeypair nkeys.KeyPair

		signingKey string
		issuerAcct string

		name      string
		nexus     string
		tags      map[string]string
		nodeState models.NodeState

		// Embedded agents
		embeddedRunners []*sdk.Runner
		// Config based agents
		localRunners []*internal.AgentProcess
		agentWatcher *internal.AgentWatcher

		minter     models.CredVendor
		state      models.NexNodeState
		auctioneer models.Auctioneer
		idgen      models.IDGen
		aregistrar models.AgentRegistrar

		regs *models.Regs

		nc          *nats.Conn
		service     micro.Service
		jsCtx       jetstream.JetStream
		server      *server.Server
		serverCreds *models.NatsConnectionData

		nodeShutdown chan struct{}
	}
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

		name:  "nexnode",
		nexus: "nexus",
		tags: map[string]string{
			models.TagOS:       runtime.GOOS,
			models.TagArch:     runtime.GOARCH,
			models.TagCPUs:     strconv.Itoa(runtime.GOMAXPROCS(0)),
			models.TagLameDuck: "false",
		},
		nodeState: models.NodeStateStarting,

		embeddedRunners: make([]*sdk.Runner, 0),
		localRunners:    make([]*internal.AgentProcess, 0),

		minter:     &credentials.FullAccessMinter{NatsServers: []string{nats.DefaultURL}},
		state:      &state.NoState{},
		auctioneer: nil,
		idgen:      idgen.NewNuidGen(),
		aregistrar: &aregistrar.AllowAllRegistrar{},

		nodeKeypair:            kp,
		nodeXKeypair:           xkp,
		allowAgentRegistration: false,
		auctionMap:             internal.NewTTLMap(time.Second * 10),

		nc:     nil,
		server: nil,

		nodeShutdown: make(chan struct{}, 1),
	}

	n.regs = models.NewRegistrationList(n.logger)

	var errs error
	for _, opt := range opts {
		if opt != nil {
			errs = errors.Join(errs, opt(n))
		}
	}
	if errs != nil {
		return nil, errs
	}

	n.ctx, n.cancel = context.WithCancel(n.ctx)
	n.tags[models.TagNexus] = n.nexus
	n.tags[models.TagNodeName] = n.name
	n.agentWatcher = internal.NewAgentWatcher(n.ctx, n.logger.WithGroup("agent-watcher"), 3)

	return n, nil
}

func (n *NexNode) agentCount() int {
	return n.regs.Count()
}

func (n *NexNode) Start() error {
	pubKey, err := n.nodeKeypair.PublicKey()
	if err != nil {
		return err
	}

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

	if n.nc == nil {
		if n.server != nil {
			n.nc, err = configureNatsConnection(n.serverCreds)
			if err != nil {
				return err
			}
		} else {
			n.nc, err = nats.Connect(nats.DefaultURL)
			if err != nil {
				return err
			}
		}
	}

	n.startTime = time.Now()
	n.logger.Info("Starting nex node",
		slog.String("version", n.version),
		slog.String("commit", n.commit),
		slog.String("build_date", n.builddate),
		slog.String("node_id", pubKey),
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
	errs = errors.Join(errs, n.service.AddEndpoint("PingNexus", micro.HandlerFunc(n.handlePing()), micro.WithEndpointSubject(models.PingSubscribeSubject()), micro.WithEndpointQueueGroup(pubKey)))
	errs = errors.Join(errs, n.service.AddEndpoint("PingNode", micro.HandlerFunc(n.handlePing()), micro.WithEndpointSubject(models.DirectPingSubscribeSubject(pubKey)), micro.WithEndpointQueueGroup(pubKey)))
	errs = errors.Join(errs, n.service.AddEndpoint("GetNodeInfo", micro.HandlerFunc(n.handleNodeInfo()), micro.WithEndpointSubject(models.NodeInfoSubscribeSubject(pubKey)), micro.WithEndpointQueueGroup(pubKey)))
	errs = errors.Join(errs, n.service.AddEndpoint("SetLameduck", micro.HandlerFunc(n.handleLameduck()), micro.WithEndpointSubject(models.LameduckSubscribeSubject(pubKey)), micro.WithEndpointQueueGroup(pubKey)))
	errs = errors.Join(errs, n.service.AddEndpoint("GetAgentIdByName", micro.HandlerFunc(n.handleGetAgentIdByName()), micro.WithEndpointSubject(models.GetAgentIdByNameSubject(pubKey)), micro.WithEndpointQueueGroup(pubKey)))
	// System only agent endpoints
	if n.allowAgentRegistration {
		errs = errors.Join(errs, n.service.AddEndpoint("InitRegisterRemoteAgent", micro.HandlerFunc(n.handleInitRegisterRemoteAgent()), micro.WithEndpointSubject(models.AgentAPIInitRemoteRegisterSubscribeSubject(n.nexus)), micro.WithEndpointQueueGroup(n.nexus)))
		// errs = errors.Join(errs, n.service.AddEndpoint("StartAgent", micro.HandlerFunc(n.handleStartAgent()), micro.WithEndpointSubject(models.StartAgentSubject(pubKey)), micro.WithEndpointQueueGroup(pubKey)))
		// errs = errors.Join(errs, n.service.AddEndpoint("StopAgent", micro.HandlerFunc(n.handleStopAgent()), micro.WithEndpointSubject(models.StopAgentSubject(pubKey)), micro.WithEndpointQueueGroup(pubKey)))
	}
	errs = errors.Join(errs, n.service.AddEndpoint("RegisterAgent", micro.HandlerFunc(n.handleRegisterAgent()), micro.WithEndpointSubject(models.AgentAPIRegisterSubscribeSubject(pubKey)), micro.WithEndpointQueueGroup(pubKey)))
	// User endpoints
	errs = errors.Join(errs, n.service.AddEndpoint("AuctionRequest", micro.HandlerFunc(n.handleAuction()), micro.WithEndpointSubject(models.AuctionSubscribeSubject()), micro.WithEndpointQueueGroup(pubKey)))
	errs = errors.Join(errs, n.service.AddEndpoint("StopWorkload", micro.HandlerFunc(n.handleStopWorkload()), micro.WithEndpointSubject(models.UndeploySubscribeSubject()), micro.WithEndpointQueueGroup(pubKey)))
	errs = errors.Join(errs, n.service.AddEndpoint("AuctionDeployWorkload", micro.HandlerFunc(n.handleAuctionDeployWorkload()), micro.WithEndpointSubject(models.AuctionDeploySubscribeSubject()), micro.WithEndpointQueueGroup(pubKey)))
	errs = errors.Join(errs, n.service.AddEndpoint("CloneWorkload", micro.HandlerFunc(n.handleCloneWorkload()), micro.WithEndpointSubject(models.CloneWorkloadSubscribeSubject()), micro.WithEndpointQueueGroup(pubKey)))
	errs = errors.Join(errs, n.service.AddEndpoint("NamespacePingRequest", micro.HandlerFunc(n.handleNamespacePing()), micro.WithEndpointSubject(models.NamespacePingSubscribeSubject()), micro.WithEndpointQueueGroup(pubKey)))

	if errs != nil {
		return errs
	}

	// At this point, nex controller is running
	err = emitSystemEvent(n.nc, n.nodeKeypair, &models.NexNodeStartedEvent{
		Id:    pubKey,
		Name:  n.name,
		Nexus: n.nexus,
		Tags:  n.tags,
		Type:  "io.synadia.nex.event.nexnode_started",
	})
	if err != nil {
		n.logger.Error("failed to emit nex started event", slog.String("err", err.Error()))
	}

	for _, e := range n.service.Info().Endpoints {
		if e.QueueGroup != micro.DefaultQueueGroup {
			n.logger.Debug("Subscribed to nats subject", slog.String("subject", e.Subject), slog.String("queue_group", e.QueueGroup))
		} else {
			n.logger.Debug("Subscribed to nats subject", slog.String("subject", e.Subject))
		}
	}

	startedRunners := []*sdk.Runner{}
	for _, runner := range n.embeddedRunners {
		if _, _, ok := n.regs.Find(runner.String()); ok {
			n.logger.Warn("agent already registered; skipping additional registration", slog.String("agent_name", runner.String()))
			continue
		}

		id := n.idgen.Generate(nil)
		err = n.regs.New(id, models.RegTypeEmbeddedAgent, "")
		if err != nil {
			n.logger.Error("failed to register agent", slog.String("agent_name", runner.String()), slog.String("err", err.Error()))
			continue
		}

		connData, err := n.minter.MintRegister(id, pubKey)
		if err != nil {
			n.logger.Error("failed to mint register", slog.String("err", err.Error()))
			continue
		}

		err = runner.Run(id, *connData)
		if err != nil {
			n.logger.Error("agent failed to run", slog.String("agent_name", runner.String()), slog.String("err", err.Error()))
			n.regs.Remove(id)
			continue
		}
		n.logger.Info("starting embedded agent", slog.String("agent_name", runner.String()))
		startedRunners = append(startedRunners, runner)
	}
	n.embeddedRunners = startedRunners

	// start local agents
	for _, agentProcess := range n.localRunners {
		agentProcess.HostNode = pubKey
		agentProcess.Id = n.idgen.Generate(nil)
		connData, err := n.minter.MintRegister(agentProcess.Id, pubKey)
		if err != nil {
			n.logger.Error("failed to mint register", slog.String("err", err.Error()))
			continue
		}
		go n.agentWatcher.New(n.regs, agentProcess, connData)
	}

	go func() {
		for range time.Tick(10 * time.Second) {
			if n.nc.IsClosed() {
				return
			}
			// TODO: what should go in this payload??
			hb := struct {
				Registrations string `json:"registrations"`
			}{
				Registrations: n.regs.String(),
			}
			hbB, err := json.Marshal(hb)
			if err != nil {
				n.logger.Error("failed Marshal heartbeat", slog.String("err", err.Error()))
			}
			err = n.nc.Publish(models.NodeEmitHeartbeatSubject(pubKey), hbB)
			if err != nil {
				n.logger.Error("failed to publish heartbeat", slog.String("err", err.Error()))
			}
		}
	}()

	time.Sleep(time.Second) // allow for local registrations to finish

	if n.regs.Count() == 0 {
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

	n.nodeState = models.NodeStateStopping

	var err error
	for _, agent := range n.embeddedRunners {
		err = agent.Shutdown()
		if err != nil {
			n.logger.Error("agent failed to shutdown", slog.String("agent_name", agent.String()), slog.String("err", err.Error()))
			continue
		}
		n.logger.Info("agent shutdown", slog.String("agent_name", agent.String()))
	}

	for _, agent := range n.regs.Items() {
		if agent.Type == models.RegTypeEmbeddedAgent {
			err = internal.StopProcess(agent.Process)
			if err == nil {
				procState, err := agent.Process.Wait()
				if err != nil {
					n.logger.Error("failed to cleanly shutdown local agent process; force killing", slog.String("err", err.Error()))
					err = agent.Process.Kill()
					if err != nil {
						n.logger.Error("failed to kill local agent process", slog.String("err", err.Error()))
						break
					}
				}
				n.logger.Info("local agent shutdown", slog.String("agent_name", agent.OriginalRequest.Name), slog.String("agent_type", agent.OriginalRequest.RegisterType), slog.String("exit_code", procState.String()))
			} else {
				n.logger.Error("failed to cleanly shutdown local agent process; force killing", slog.String("err", err.Error()))
				err = agent.Process.Kill()
				if err != nil {
					n.logger.Error("failed to kill local agent process", slog.String("err", err.Error()), slog.Int("pid", agent.Process.Pid))
					break
				}
			}
		}
	}

	err = n.service.Stop()
	if err != nil {
		n.logger.Error("failed to stop micro service", slog.String("err", err.Error()))
	}
	if !n.nc.IsClosed() {
		err = n.nc.Drain()
		if err != nil {
			n.logger.Error("failed to drain nats connection", slog.String("err", err.Error()))
		}
	}

	n.logger.Info("nex node stopped", slog.String("uptime", time.Since(n.startTime).String()))

	n.nodeShutdown <- struct{}{}
	return nil
}

func (n *NexNode) WaitForShutdown() error {
	for {
		select {
		case <-n.ctx.Done(): // shutdown by context
			n.logger.Warn("shutdown by context cancellation")
			return n.Shutdown()
		case <-n.nodeShutdown: // shutdown by command, recommended
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
}

func configureNatsConnection(connData *models.NatsConnectionData) (*nats.Conn, error) {
	if connData.ConnName == "" {
		connData.ConnName = "nexnode"
	}

	opts := []nats.Option{
		nats.Name(connData.ConnName),
		nats.MaxReconnects(-1),
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
