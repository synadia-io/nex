package actors

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	agentcommon "github.com/synadia-io/nex/agents/common"
	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"

	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
)

const (
	registrationTimeout = 5 * time.Second
)

// The ExternalAgent is responsible for managing and communicating with a single external agent.
// It is responsible for starting the executable and passing the `agentBinaryCreds` to the agent
// via environment variables, while the actor itself connects to the internal NATS via the
// `internalNatsUrl` and the `hostUserKeypair`
type ExternalAgent struct {
	agentOptions     models.AgentOptions
	agentBinaryCreds AgentCredential
	hostUserKeypair  nkeys.KeyPair
	logger           *slog.Logger
	self             *goakt.PID
	conn             *nats.Conn
	agentClient      *agentcommon.AgentClient
	internalNatsUrl  string
	nodeOptions      *models.NodeOptions
}

func CreateExternalAgent(logger *slog.Logger,
	agentBinaryCreds AgentCredential,
	hostUserKeyPair nkeys.KeyPair,
	agentOptions models.AgentOptions,
	nodeOptions *models.NodeOptions) *ExternalAgent {

	return &ExternalAgent{
		agentOptions:     agentOptions,
		agentBinaryCreds: agentBinaryCreds,
		hostUserKeypair:  hostUserKeyPair,
		logger:           logger,
		nodeOptions:      nodeOptions,
	}
}

func (a *ExternalAgent) PreStart(ctx context.Context) error {
	return nil
}

func (a *ExternalAgent) PostStop(ctx context.Context) error {
	return nil
}

func (a *ExternalAgent) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goaktpb.PostStart:
		if err := a.startBinary(); err != nil {
			ctx.Err(errors.Join(errors.New("Failed to load and start agent binary"), err))
			return
		}
		a.self = ctx.Self()
		_ = a.self.ActorSystem().ScheduleOnce(
			context.Background(),
			&actorproto.CheckRegistered{},
			a.self,
			registrationTimeout)
		a.logger.Info("External agent for workload type is running", slog.String("name", ctx.Self().Name()))
	case *actorproto.AgentRegistered:
		err := a.createAgentClient(msg.InternalNatsUrl, msg.InternalNkey, msg.InternalNkeySeed)
		if err != nil {
			ctx.Err(err)
			return
		}
		ctx.Become(a.RegisteredAgentReceive)
	case *actorproto.CheckRegistered:
		a.logger.Error("Agent was not registered within timeout period. Agent actor terminating", slog.String("agent", a.agentOptions.Name))
		ctx.Stop(a.self)
	default:
		ctx.Unhandled()
	}
}

func (a *ExternalAgent) RegisteredAgentReceive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actorproto.QueryWorkloads:
		a.queryWorkloads(ctx)
	case *actorproto.StartWorkload:
		a.startWorkload(ctx, msg)
	default:
		ctx.Unhandled()
	}
}

func (a *ExternalAgent) startWorkload(ctx *goakt.ReceiveContext, req *actorproto.StartWorkload) {
	// TODO: send start workload request to agent

	// TODO: handle result (ctx.Error, etc)
}

func (a *ExternalAgent) startBinary() error {
	// TODO: download the artifact, create an OsProcess, run it

	u, err := url.Parse(a.internalNatsUrl)
	if err != nil {
		return err
	}
	seed, err := a.agentBinaryCreds.nkey.Seed()
	if err != nil {
		return err
	}
	env := make(map[string]string)
	env[agentcommon.EnvNatsHost] = u.Host
	env[agentcommon.EnvNatsPort] = u.Port()
	env[agentcommon.EnvNatsNkey] = string(seed)

	// TODO: Note that host services information needs to be passed to the agent in start
	// workload request because the Hs connections are per-workload

	return nil
}

func (a *ExternalAgent) createAgentClient(url string, _ string, seed string) error {
	var err error
	opt, err := nats.NkeyOptionFromSeed(seed)
	if err != nil {
		a.logger.Error("Failed to extract an nkey option from the nkey seed", slog.Any("error", err))
		return err
	}
	a.conn, err = nats.Connect(url, opt, nats.Name(a.agentOptions.Name))
	if err != nil {
		a.logger.Error("Failed to create a connection to internal NATS server for agent", slog.String("agent", a.agentOptions.Name))
		return err
	}
	a.internalNatsUrl = url
	a.logger.Debug("External agent actor connected to internal NATS", slog.String("agent", a.agentOptions.Name))

	a.agentClient, err = agentcommon.NewAgentClient(a.conn, a.agentOptions.Name)
	if err != nil {
		a.logger.Error("Failed to create agent client", slog.Any("error", err), slog.String("agent", a.agentOptions.Name))
		return err
	}
	return nil
}

func (a *ExternalAgent) queryWorkloads(ctx *goakt.ReceiveContext) {
	// TODO: make this real
}
