package actors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"

	nodecontrol "github.com/synadia-io/nex/api/nodecontrol/gen"
	"github.com/synadia-io/nex/models"
	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
)

const ControlAPIActorName = "control_api"

const (
	AuctionResponseType      = "io.nats.nex.v2.auction_response"
	InfoResponseType         = "io.nats.nex.v2.info_response"
	PingResponseType         = "io.nats.nex.v2.ping_response"
	AgentPingResponseType    = "io.nats.nex.v2.agent_ping_response"
	WorkloadPingResponseType = "io.nats.nex.v2.workload_ping_response"
	RunResponseType          = "io.nats.nex.v2.run_response"
	StopResponseType         = "io.nats.nex.v2.stop_response"
	LameDuckResponseType     = "io.nats.nex.v2.lameduck_response"
)

type auctionResponseFunc func(string, string, []string, map[string]string) (*actorproto.AuctionResponse, error)
type nodePingResponseFunc func() (*actorproto.PingNodeResponse, error)
type nodeInfoResponseFunc func() (*actorproto.NodeInfo, error)
type nodeLameDuckFunc func(context.Context)

type ControlAPI struct {
	nc         *nats.Conn
	logger     *slog.Logger
	publicKey  string
	publicXKey string
	subsz      []*nats.Subscription

	auctionResponse  auctionResponseFunc
	nodePingResponse nodePingResponseFunc
	nodeInfoResponse nodeInfoResponseFunc
	nodeLameDuck     nodeLameDuckFunc

	self *goakt.PID
}

func CreateControlAPI(nc *nats.Conn, logger *slog.Logger, publicKey string, aF auctionResponseFunc, nF nodePingResponseFunc, iF nodeInfoResponseFunc, ldF nodeLameDuckFunc) *ControlAPI {
	api := &ControlAPI{
		nc:               nc,
		logger:           logger,
		publicKey:        publicKey,
		subsz:            make([]*nats.Subscription, 0),
		auctionResponse:  aF,
		nodePingResponse: nF,
		nodeInfoResponse: iF,
		nodeLameDuck:     ldF,
	}

	kp, err := nkeys.CreateCurveKeys()
	if err != nil {
		logger.Error("Failed to create curve keys", slog.Any("error", err))
		return nil
	}

	api.publicXKey, err = kp.PublicKey()
	if err != nil {
		logger.Error("Failed to get public key", slog.Any("error", err))
		return nil
	}

	return api
}

func (a *ControlAPI) PreStart(ctx context.Context) error {
	return nil
}

func (a *ControlAPI) PostStop(ctx context.Context) error {
	for _, sub := range a.subsz {
		_ = sub.Unsubscribe()
	}

	return nil
}

func (a *ControlAPI) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		a.self = ctx.Self()
		err := a.subscribe()
		if err != nil {
			_ = a.shutdown()
			ctx.Err(err)
		}
		a.logger.Info("Control API NATS server is running", slog.String("name", ctx.Self().Name()))
	default:
		ctx.Unhandled()
	}
}

func (api *ControlAPI) shutdown() error {
	var err error

	for _, sub := range api.subsz {
		if !sub.IsValid() {
			continue
		}

		_err := sub.Drain()
		if _err != nil {
			api.logger.Warn("Failed to drain API subscription", slog.String("subscription", sub.Subject))
			err = errors.Join(_err, fmt.Errorf("failed to drain API subscription: %s", sub.Subject))
		}
	}

	return err
}

func (api *ControlAPI) subscribe() error {
	var sub *nats.Subscription
	var err error

	sub, err = api.nc.Subscribe(models.AuctionSubject(), api.handleAuction)
	if err != nil {
		api.logger.Error("Failed to subscribe to auction subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(DeploySubscribeSubject(api.publicKey), api.handleDeploy)
	if err != nil {
		api.logger.Error("Failed to subscribe to run subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(InfoSubscribeSubject(api.publicKey), api.handleInfo)
	if err != nil {
		api.logger.Error("Failed to subscribe to info subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(models.LameduckSubject(api.publicKey), api.handleLameDuck)
	if err != nil {
		api.logger.Error("Failed to subscribe to lame duck subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(models.PingSubject(), api.handlePing)
	if err != nil {
		api.logger.Error("Failed to subscribe to ping subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(models.DirectPingSubject(api.publicKey), api.handlePing)
	if err != nil {
		api.logger.Error("Failed to subscribe to node-specific ping subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(UndeploySubscribeSubject(api.publicKey), api.handleUndeploy)
	if err != nil {
		api.logger.Error("Failed to subscribe to undeploy subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(models.AgentPingSubscribeSubject(), api.handleAgentPing)
	if err != nil {
		api.logger.Error("Failed to subscribe to agent ping subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	api.logger.Info("NATS execution engine awaiting commands", slog.String("id", api.publicKey)) //slog.String("version", VERSION)
	return nil
}

func (api *ControlAPI) handleAuction(m *nats.Msg) {
	req := new(nodecontrol.AuctionRequestJson)
	err := json.Unmarshal(m.Data, req)
	if err != nil {
		api.logger.Error("Failed to unmarshal auction request", slog.Any("error", err))
		models.RespondEnvelope(m, AuctionResponseType, 500, "", fmt.Sprintf("failed to unmarshal auction request: %s", err))
		return
	}

	convertedAgentType := make([]string, len(req.AgentType))
	for _, at := range req.AgentType {
		convertedAgentType = append(convertedAgentType, string(at))
	}

	auctResp, err := api.auctionResponse(string(req.Os), string(req.Arch), convertedAgentType, req.Tags.Tags)
	if err != nil {
		api.logger.Error("Failed to generate auction response", slog.Any("error", err))
		models.RespondEnvelope(m, AuctionResponseType, 500, "", fmt.Sprintf("failed to generate auction response: %s", err))
		return
	}

	if auctResp == nil {
		return
	}

	models.RespondEnvelope(m, AuctionResponseType, 200, auctionResponseFromProto(auctResp), "")
}

func (api *ControlAPI) handleDeploy(m *nats.Msg) {
	req := new(nodecontrol.StartWorkloadRequestJson)
	err := json.Unmarshal(m.Data, req)
	if err != nil {
		api.logger.Error("Failed to unmarshal deploy request", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to unmarshal deploy request: %s", err))
		return
	}

	ctx := context.Background()
	_, agent, err := api.self.ActorSystem().ActorOf(ctx, req.WorkloadType)
	if err != nil {
		api.logger.Error("Failed to locate agent actor", slog.String("type", req.WorkloadType), slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to locate [%s] agent actor: %s", req.WorkloadType, err))
		return
	}

	askResp, err := api.self.Ask(ctx, agent, startRequestToProto(req))
	if err != nil {
		api.logger.Error("Failed to start workload", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("Failed to start workload: %s", err))
		return
	}

	protoResp, ok := askResp.(*actorproto.WorkloadStarted)
	if !ok {
		api.logger.Error("Start workload response from agent was not the correct type")
		models.RespondEnvelope(m, RunResponseType, 500, "", "Agent returned the wrong data type")
		return
	}

	models.RespondEnvelope(m, RunResponseType, 200, startResponseFromProto(protoResp), "")
}

func (api *ControlAPI) handleUndeploy(m *nats.Msg) {
	req := new(nodecontrol.StopWorkloadRequestJson)
	err := json.Unmarshal(m.Data, req)
	if err != nil {
		api.logger.Error("Failed to unmarshal undeploy request", slog.Any("error", err))
		models.RespondEnvelope(m, StopResponseType, 500, "", fmt.Sprintf("failed to unmarshal undeploy request: %s", err))
		return
	}

	ctx := context.Background()
	_, agent, err := api.self.ActorSystem().ActorOf(ctx, req.WorkloadType)
	if err != nil {
		api.logger.Error("Failed to locate agent actor", slog.String("type", req.WorkloadType), slog.Any("error", err))
		models.RespondEnvelope(m, StopResponseType, 500, "", fmt.Sprintf("failed to locate agent [%s] actor: %s", req.WorkloadType, err))
		return
	}

	askResp, err := api.self.Ask(ctx, agent, stopRequestToProto(req))
	if err != nil {
		api.logger.Error("Failed to stop workload on agent", slog.String("agent", agent.Name()), slog.Any("error", err))
		models.RespondEnvelope(m, StopResponseType, 500, "", fmt.Sprintf("failed to locate agent supervisor actor: %s", err))
		return
	}

	protoResp, ok := askResp.(*actorproto.WorkloadStopped)
	if !ok {
		api.logger.Error("Workload stop response from agent was not the correct type")
		models.RespondEnvelope(m, StopResponseType, 500, "", "Agent returned the wrong data type for workload stop")
		return
	}

	models.RespondEnvelope(m, StopResponseType, 200, stopResponseFromProto(protoResp), "")
}

func (api *ControlAPI) handleInfo(m *nats.Msg) {
	info, err := api.nodeInfoResponse()
	if err != nil {
		api.logger.Error("Failed to get node info", slog.Any("error", err))
		models.RespondEnvelope(m, InfoResponseType, 500, "", fmt.Sprintf("failed to get node info: %s", err))
		return
	}

	models.RespondEnvelope(m, InfoResponseType, 200, infoResponseFromProto(info), "")
}

func (api *ControlAPI) handleLameDuck(m *nats.Msg) {
	// Set a timeout for the lame duck request of 1 minute
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	api.nodeLameDuck(ctx)

	api.logger.Info("Received lame duck request")
	_, agentSuper, err := api.self.ActorSystem().ActorOf(ctx, AgentSupervisorActorName)
	if err != nil {
		api.logger.Error("Failed to locate agent supervisor actor", slog.Any("error", err))
		models.RespondEnvelope(m, LameDuckResponseType, 500, "", fmt.Sprintf("failed to locate agent supervisor actor: %s", err))
		return
	}

	response, err := api.self.Ask(ctx, agentSuper, new(actorproto.SetLameDuck))
	if err != nil {
		api.logger.Error("Failed to put node in lame duck mode", slog.Any("error", err))
		models.RespondEnvelope(m, InfoResponseType, 500, "", fmt.Sprintf("Failed to put node in lame duck mode: %s", err))
		return
	}

	workloadResponse, ok := response.(*actorproto.LameDuckResponse)
	if !ok {
		api.logger.Error("LameDuck response from agent supervisor was not the correct type")
		models.RespondEnvelope(m, LameDuckResponseType, 500, "", "LameDuck response from agent supervisor was not the correct type")
		return
	}

	if !workloadResponse.Success {
		api.logger.Error("Failed to put node in lame duck mode")
		models.RespondEnvelope(m, LameDuckResponseType, 500, "", "Failed to put node in lame duck mode")
		return
	}

	workloadResponse.Id = api.publicKey
	models.RespondEnvelope(m, LameDuckResponseType, 200, lameDuckResponseFromProto(workloadResponse), "")
}

func (api *ControlAPI) handlePing(m *nats.Msg) {
	pingResponse, err := api.nodePingResponse()
	if err != nil {
		api.logger.Error("failed to ping node", slog.Any("error", err))
		models.RespondEnvelope(m, PingResponseType, 500, "", fmt.Sprintf("failed to ping node: %s", err.Error()))
		return
	}

	models.RespondEnvelope(m, PingResponseType, 200, pingResponseFromProto(pingResponse), "")
}

func (api *ControlAPI) handleAgentPing(m *nats.Msg) {
	ctx := context.Background()

	splitSub := strings.Split(m.Subject, ".")
	var workloadType, namespace, workloadID string

	switch len(splitSub) {
	case 4: // PREFIX.APING.<WORKLOAD_TYPE>.<NAMESPACE>
		workloadType = splitSub[2]
		namespace = splitSub[3]

		_, agent, err := api.self.ActorSystem().ActorOf(ctx, workloadType)
		if err != nil {
			api.logger.Error("Failed to locate agent actor", slog.String("type", workloadType), slog.Any("error", err))
			models.RespondEnvelope(m, AgentPingResponseType, 500, "", fmt.Sprintf("failed to locate agent [%s] actor: %s", workloadType, err))
			return
		}

		response, err := api.self.Ask(ctx, agent, &actorproto.PingAgent{
			Type:      workloadType,
			Namespace: namespace,
		})
		if err != nil {
			api.logger.Error("failed to ping agent", slog.Any("error", err))
			models.RespondEnvelope(m, AgentPingResponseType, 500, "", fmt.Sprintf("failed to ping agent: %s", err))
			return
		}

		aPingResponse, ok := response.(*actorproto.PingAgentResponse)
		if !ok {
			api.logger.Error("Response from agent ping was not the correct type")
			models.RespondEnvelope(m, AgentPingResponseType, 500, "", "Response from agent ping was not the correct type")
			return
		}

		models.RespondEnvelope(m, AgentPingResponseType, 200, agentPingResponseFromProto(aPingResponse), "")
	case 5: // PREFIX.APING.<NAMESPACE>.<WORKLOAD_ID>
		workloadType = splitSub[2]
		namespace = splitSub[3]
		workloadID = splitSub[4]

		_, agent, err := api.self.ActorSystem().ActorOf(ctx, workloadType)
		if err != nil {
			api.logger.Error("Failed to locate agent actor", slog.String("type", workloadType), slog.Any("error", err))
			models.RespondEnvelope(m, WorkloadPingResponseType, 500, "", fmt.Sprintf("failed to locate agent [%s] actor: %s", workloadType, err))
			return
		}

		response, err := api.self.Ask(ctx, agent, &actorproto.PingWorkload{
			Type:       workloadType,
			Namespace:  namespace,
			WorkloadId: workloadID,
		})
		if err != nil {
			api.logger.Error("failed to ping workload", slog.Any("error", err))
			models.RespondEnvelope(m, WorkloadPingResponseType, 500, "", fmt.Sprintf("failed to ping workload: %s", err))
			return
		}

		aWorkloadPingResponse, ok := response.(*actorproto.PingWorkloadResponse)
		if !ok {
			api.logger.Error("Response from workload ping was not the correct type")
			models.RespondEnvelope(m, WorkloadPingResponseType, 500, "", "Response from workload ping was not the correct type")
			return
		}

		models.RespondEnvelope(m, WorkloadPingResponseType, 200, workloadPingResponseFromProto(aWorkloadPingResponse), "")

	default:
		api.logger.Error("Received a request on a bad APING subject", slog.Any("subjet", m.Subject))
		models.RespondEnvelope(m, AgentPingResponseType, 500, "", "Received a request on a bad APING subject")
		return
	}
}
