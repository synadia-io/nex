package actors

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/nats-io/nuid"
	agentcommon "github.com/synadia-io/nex/agents/common"
	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"

	agentapi "github.com/synadia-io/nex/api/agent/go"
	agentapigen "github.com/synadia-io/nex/api/agent/go/gen"
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
	agentOptions              models.AgentOptions
	agentBinaryCreds          AgentCredential
	hostUserKeypair           nkeys.KeyPair
	logger                    *slog.Logger
	self                      *goakt.PID
	internalConn              *nats.Conn
	controlConn               *nats.Conn
	workloadHostServicesConns map[string]*nats.Conn
	agentClient               *agentcommon.AgentClient
	internalNatsUrl           string
	nodeOptions               *models.NodeOptions
	subz                      map[string][]*nats.Subscription
}

func CreateExternalAgent(logger *slog.Logger,
	agentBinaryCreds AgentCredential,
	hostUserKeyPair nkeys.KeyPair,
	internalNatsUrl string,
	agentOptions models.AgentOptions,
	nc *nats.Conn,
	nodeOptions *models.NodeOptions) *ExternalAgent {

	return &ExternalAgent{
		agentOptions:              agentOptions,
		agentBinaryCreds:          agentBinaryCreds,
		hostUserKeypair:           hostUserKeyPair,
		logger:                    logger,
		internalNatsUrl:           internalNatsUrl,
		nodeOptions:               nodeOptions,
		controlConn:               nc,
		workloadHostServicesConns: make(map[string]*nats.Conn),
		subz:                      make(map[string][]*nats.Subscription),
	}
}

func (a *ExternalAgent) PreStart(ctx context.Context) error {
	return nil
}

func (a *ExternalAgent) PostStop(ctx context.Context) error {
	a.logger.Debug("External agent actor stopped", slog.String("agent_name", a.agentOptions.Name))
	return nil
}

// Message handlers that apply to the pre-registration state
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

// Message handlers that apply to the post-registration state
func (a *ExternalAgent) RegisteredAgentReceive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actorproto.QueryWorkloads:
		a.queryWorkloads(ctx)
	case *actorproto.StartWorkload:
		a.startWorkload(ctx, msg)
	case *actorproto.StopWorkload:
		a.stopWorkload(ctx, msg)
	// NOTE: we don't respond to a trigger workload here because this agent actor
	// is directly subscribing to the trigger subjects for all workloads of this type
	default:
		ctx.Unhandled()
	}
}

func (a *ExternalAgent) stopWorkload(ctx *goakt.ReceiveContext, req *actorproto.StopWorkload) {
	err := a.agentClient.StopWorkload(req.WorkloadId)
	if err != nil {
		a.logger.Error("Failed to stop workload",
			slog.String("agent_name", a.agentOptions.Name),
			slog.Any("error", err))
		return
	}
	for _, sub := range a.subz[req.WorkloadId] {
		_ = sub.Unsubscribe()
	}
	if hsConn, ok := a.workloadHostServicesConns[req.WorkloadId]; ok {
		_ = hsConn.Drain()
		delete(a.workloadHostServicesConns, req.WorkloadId)
	}
}

func (a *ExternalAgent) startWorkload(ctx *goakt.ReceiveContext, req *actorproto.StartWorkload) {
	artRef, err := GetArtifact(req.WorkloadName, req.Uri, a.controlConn)
	if err != nil {
		a.logger.Error("Failed to retrieve artifact for workload",
			slog.String("agent_name", a.agentOptions.Name),
			slog.String("uri", req.Uri),
			slog.String("workload_name", req.WorkloadName),
			slog.Any("error", err))
		return
	}
	reqJson := &agentapigen.StartWorkloadRequestJson{
		LocalFilePath:   artRef.LocalCachePath,
		Argv:            req.Argv,
		Env:             nil, // TODO: provide a means for the external agent to decrypt this, or decrypt it prior to sending the request
		Hash:            req.Hash,
		Name:            req.WorkloadName,
		Namespace:       req.Namespace,
		TotalBytes:      0, // TODO: see if we still need this?
		TriggerSubjects: req.TriggerSubjects,
		WorkloadId:      nuid.Next(),
		WorkloadType:    req.WorkloadType,
	}
	hsConn, err := a.createHostServicesConnection(req)
	if err != nil {
		a.logger.Error("Failed to create connection to host services for workload",
			slog.String("agent_name", a.agentOptions.Name),
			slog.String("workload_name", req.WorkloadName),
			slog.Any("error", err))
		return
	}
	a.workloadHostServicesConns[reqJson.WorkloadId] = hsConn

	// TODO: remove magic string
	if req.WorkloadType != agentapi.WorkloadTypeDirect &&
		req.WorkloadType != agentapi.WorkloadTypeMicroVM &&
		len(req.TriggerSubjects) > 0 {
		err := a.subscribeToTriggerSubjects(req, reqJson.WorkloadId, hsConn)
		if err != nil {
			a.logger.Error("Failed to subscribe to trigger subjects on host services connection",
				slog.String("agent_name", a.agentOptions.Name),
				slog.String("workload_name", req.WorkloadName),
				slog.Any("error", err))
			return
		}
	}

	err = a.agentClient.StartWorkload(reqJson)
	if err != nil {
		a.logger.Error("Failed to start workload",
			slog.String("agent_name", a.agentOptions.Name),
			slog.Any("error", err))
		return
	}
}

func (a *ExternalAgent) createHostServicesConnection(request *actorproto.StartWorkload) (*nats.Conn, error) {
	natsOpts := []nats.Option{}
	if len(strings.TrimSpace(request.WorkloadName)) > 0 {
		natsOpts = append(natsOpts, nats.Name("nex-hostservices-"+strings.ToLower(request.WorkloadName)))
	} else {
		natsOpts = append(natsOpts, nats.Name("nex-hostservices"))
	}
	var url string
	if request.HostServiceConfig != nil {
		natsOpts = append(natsOpts,
			nats.UserJWTAndSeed(request.HostServiceConfig.NatsUserJwt,
				request.HostServiceConfig.NatsUserSeed))
		url = request.HostServiceConfig.NatsUrl
	} else if len(strings.TrimSpace(a.nodeOptions.HostServiceOptions.NatsUserSeed)) > 0 {
		natsOpts = append(natsOpts,
			nats.UserJWTAndSeed(a.nodeOptions.HostServiceOptions.NatsUserJwt,
				a.nodeOptions.HostServiceOptions.NatsUserSeed))
		if len(strings.TrimSpace(a.nodeOptions.HostServiceOptions.NatsUrl)) != 0 {
			url = a.nodeOptions.HostServiceOptions.NatsUrl
		} else {
			url = a.controlConn.Servers()[0]
		}
	} else {
		if a.controlConn.Opts.UserJWT != nil {
			natsOpts = append(natsOpts,
				nats.UserJWT(a.controlConn.Opts.UserJWT, a.controlConn.Opts.SignatureCB))
		}

		url = a.controlConn.Opts.Url
	}

	a.logger.Debug("Attempting to establish host services connection for workload",
		slog.String("workload_name", request.WorkloadName),
		slog.String("url", url))

	nc, err := nats.Connect(url, natsOpts...)
	if err != nil {
		a.logger.Error("Failed to establish host services connection for workload",
			slog.String("workload_name", request.WorkloadName),
			slog.String("url", url),
			slog.Any("error", err))
		return nil, err
	}

	return nc, nil
}

func (a *ExternalAgent) subscribeToTriggerSubjects(request *actorproto.StartWorkload, workloadId string, conn *nats.Conn) error {
	for _, tsub := range request.TriggerSubjects {
		sub, err := conn.QueueSubscribe(tsub, tsub, a.generateTriggerHandler(workloadId, tsub))
		if err != nil {
			return err
		}
		a.subz[workloadId] = append(a.subz[workloadId], sub)
	}
	return nil
}

func (a *ExternalAgent) generateTriggerHandler(workloadId string, tsub string) func(m *nats.Msg) {

	agentClient := a.agentClient
	agentName := a.agentOptions.Name

	return func(msg *nats.Msg) {

		bytes, err := agentClient.TriggerWorkload(workloadId, msg.Data)
		if err != nil {
			// TODO: respond with error envelope
			a.logger.Error("Failed to trigger workload",
				slog.String("subscribe_subject", tsub),
				slog.String("receive_subject", msg.Subject),
				slog.String("agent_name", agentName),
				slog.Any("error", err))
			return
		}
		err = msg.Respond(bytes)
		if err != nil {
			a.logger.Error("Failed to respond to trigger subject request",
				slog.String("subject", tsub),
				slog.String("agent_name", agentName),
				slog.String("receive_subject", msg.Subject),
				slog.Any("error", err))
			return
		}
	}
}

func (a *ExternalAgent) startBinary() error {
	artRef, err := GetArtifact(a.agentOptions.Name, a.agentOptions.Uri, a.controlConn)
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
