package actors

import (
	"context"
	"errors"
	"fmt"
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
	internalConn     *nats.Conn
	controlConn      *nats.Conn
	agentClient      *agentcommon.AgentClient
	internalNatsUrl  string
	nodeOptions      *models.NodeOptions
}

func CreateExternalAgent(logger *slog.Logger,
	agentBinaryCreds AgentCredential,
	hostUserKeyPair nkeys.KeyPair,
	internalNatsUrl string,
	agentOptions models.AgentOptions,
	nc *nats.Conn,
	nodeOptions *models.NodeOptions) *ExternalAgent {

	return &ExternalAgent{
		agentOptions:     agentOptions,
		agentBinaryCreds: agentBinaryCreds,
		hostUserKeypair:  hostUserKeyPair,
		logger:           logger,
		internalNatsUrl:  internalNatsUrl,
		nodeOptions:      nodeOptions,
		controlConn:      nc,
	}
}

func (a *ExternalAgent) PreStart(ctx context.Context) error {
	return nil
}

func (a *ExternalAgent) PostStop(ctx context.Context) error {
	a.logger.Debug("External agent actor stopped", slog.String("agent_name", a.agentOptions.Name))
	/*
		child, err := a.self.Child(a.childActorName())
		if err == nil && child != nil && child.IsRunning() {
			err = child.Tell(context.Background(), child, &actorproto.KillDirectStartProcess{})
			if err != nil {
				a.logger.Error("failed to stop agent OS process",
					slog.String("agent_name", a.agentOptions.Name),
					slog.String("name", a.self.Name()),
					slog.Any("err", err))
				return err
			}
		}
	*/
	return nil
}

func (a *ExternalAgent) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goaktpb.PostStart:
		a.logger.Info("External agent starting", slog.String("agent_name", a.agentOptions.Name))
		a.self = ctx.Self()
		_ = a.self.Tell(context.Background(), a.self, &actorproto.SpawnAgentBinary{})
		_ = a.self.ActorSystem().ScheduleOnce(
			context.Background(),
			&actorproto.CheckRegistered{},
			a.self,
			registrationTimeout)
		a.logger.Info("External agent for workload type is running", slog.String("name", ctx.Self().Name()))
	case *actorproto.SpawnAgentBinary:
		if err := a.startBinary(); err != nil {
			a.logger.Error("Failed to load and start agent binary",
				slog.String("agent_name", a.agentOptions.Name),
				slog.Any("error", err))
			ctx.Err(errors.Join(errors.New("Failed to load and start agent binary"), err))
			return
		}

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
	artRef, err := GetArtifact(a.agentOptions.Name, a.agentOptions.Uri, a.internalConn)
	if err != nil {
		a.logger.Error("Failed to retrieve artifact",
			slog.String("agent_name", a.agentOptions.Name),
			slog.Any("error", err))
		return err
	}

	u, err := url.Parse(a.internalNatsUrl)
	if err != nil {
		a.logger.Error("Failed to parse internal NATS url")
		return err
	}

	seed, err := a.agentBinaryCreds.nkey.Seed()
	if err != nil {
		a.logger.Error("Invalid internal NATS creds", slog.Any("error", err))
		return err
	}
	env := make(map[string]string)
	env[agentcommon.EnvNatsHost] = u.Hostname()
	env[agentcommon.EnvNatsPort] = u.Port()
	env[agentcommon.EnvNatsNkey] = string(seed)

	// TODO: Note that host services information needs to be passed to the agent in start
	// workload request because the Hs connections are per-workload

	processActor, err := createNewProcessActor(
		a.logger.WithGroup(fmt.Sprintf("proc-%s", a.agentOptions.Name)),
		a.controlConn,
		a.agentOptions.Name,
		[]string{"up"},
		"agent",
		"agent",
		a.agentOptions.Name,
		artRef,
		env,
	)
	if err != nil {
		a.logger.Error("Failed to spawn OS process actor child",
			slog.Any("error", err),
			slog.String("agent_name", a.agentOptions.Name))
		return err
	}
	c, err := a.self.SpawnChild(context.Background(), a.childActorName(), processActor)
	if err != nil {
		a.logger.Error("Failed to spawn agent process",
			slog.String("name", a.self.Name()),
			slog.String("agent_name", a.agentOptions.Name),
			slog.Any("err", err))
		return err
	}

	a.logger.Info("Spawned direct start process", slog.String("name", c.Name()))

	return nil
}

func (a *ExternalAgent) createAgentClient(url string, _ string, seed string) error {
	var err error
	pair, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		a.logger.Error("Bad seed!", slog.Any("error", err))
		return err
	}
	pk, err := pair.PublicKey()
	if err != nil {
		a.logger.Error("Bad public key", slog.Any("error", err))
		return err
	}
	opt := nats.Nkey(pk, func(b []byte) ([]byte, error) {
		return pair.Sign(b)
	})
	if err != nil {
		a.logger.Error("Failed to extract an nkey option from the nkey seed", slog.Any("error", err))
		return err
	}
	a.internalConn, err = nats.Connect(url, opt, nats.Name(a.agentOptions.Name))
	if err != nil {
		a.logger.Error("Failed to create a connection to internal NATS server for agent", slog.String("agent", a.agentOptions.Name))
		return err
	}
	a.internalNatsUrl = url
	a.logger.Debug("External agent actor connected to internal NATS")

	a.agentClient, err = agentcommon.NewAgentClient(a.internalConn, a.agentOptions.Name)
	if err != nil {
		a.logger.Error("Failed to create agent client", slog.Any("error", err), slog.String("agent", a.agentOptions.Name))
		return err
	}
	return nil
}

func (a *ExternalAgent) queryWorkloads(ctx *goakt.ReceiveContext) {
	a.logger.Debug("QueryWorkloads received", slog.String("name", a.agentOptions.Name))
	// TODO: make this real
	ctx.Response(&actorproto.WorkloadList{
		Workloads: []*actorproto.WorkloadSummary{},
	})
}

func (a *ExternalAgent) childActorName() string {
	return fmt.Sprintf("agent-%s-proc", a.agentOptions.Name)
}
