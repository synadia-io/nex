package actors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"

	nodecontrol "github.com/synadia-io/nex/api/nodecontrol/gen"
	actorproto "github.com/synadia-io/nex/node/actors/pb"
)

const ControlAPIActorName = "control_api"

const (
	AuctionResponseType   = "io.nats.nex.v2.auction_response"
	InfoResponseType      = "io.nats.nex.v2.info_response"
	PingResponseType      = "io.nats.nex.v2.ping_response"
	AgentPingResponseType = "io.nats.nex.v2.agent_ping_response"
	RunResponseType       = "io.nats.nex.v2.run_response"
	StopResponseType      = "io.nats.nex.v2.stop_response"
	LameDuckResponseType  = "io.nats.nex.v2.lameduck_response"
)

type auctionResponseFunc func(string, string, []string, map[string]string) (*actorproto.AuctionResponse, error)
type ControlAPI struct {
	nc         *nats.Conn
	logger     *slog.Logger
	publicKey  string
	publicXKey string
	subsz      []*nats.Subscription

	auctionResponse auctionResponseFunc

	self *goakt.PID
}

func CreateControlAPI(nc *nats.Conn, logger *slog.Logger, publicKey string, aF auctionResponseFunc) *ControlAPI {
	api := &ControlAPI{
		nc:              nc,
		logger:          logger,
		publicKey:       publicKey,
		subsz:           make([]*nats.Subscription, 0),
		auctionResponse: aF,
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

	sub, err = api.nc.Subscribe(AuctionSubject(), api.handleAuction)
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

	sub, err = api.nc.Subscribe(LameduckSubject(api.publicKey), api.handleLameDuck)
	if err != nil {
		api.logger.Error("Failed to subscribe to lame duck subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(PingSubject(), api.handlePing)
	if err != nil {
		api.logger.Error("Failed to subscribe to ping subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(DirectPingSubject(api.publicKey), api.handlePing)
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

	sub, err = api.nc.Subscribe(AgentPingSubscribeSubject(), api.handleAgentPing)
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
		respondEnvelope(m, AuctionResponseType, 500, nil, fmt.Sprintf("failed to unmarshal auction request: %s", err))
		return
	}

	convertedAgentType := make([]string, len(req.AgentType))
	for _, at := range req.AgentType {
		convertedAgentType = append(convertedAgentType, string(at))
	}

	auctResp, err := api.auctionResponse(string(req.Os), string(req.Arch), convertedAgentType, req.Tags.Tags)
	if err != nil {
		api.logger.Error("Failed to generate auction response", slog.Any("error", err))
		respondEnvelope(m, AuctionResponseType, 500, nil, fmt.Sprintf("failed to generate auction response: %s", err))
		return
	}

	respondEnvelope(m, AuctionResponseType, 200, auctionResponseFromProto(auctResp), "")
}

func (api *ControlAPI) handleDeploy(m *nats.Msg) {
	req := new(nodecontrol.StartWorkloadRequestJson)
	err := json.Unmarshal(m.Data, req)
	if err != nil {
		api.logger.Error("Failed to unmarshal deploy request", slog.Any("error", err))
		respondEnvelope(m, RunResponseType, 500, nil, fmt.Sprintf("failed to unmarshal deploy request: %s", err))
		return
	}

	ctx := context.Background()
	_, agentSuper, err := api.self.ActorSystem().ActorOf(ctx, AgentSupervisorActorName)
	if err != nil {
		api.logger.Error("Failed to locate agent supervisor actor", slog.Any("error", err))
		respondEnvelope(m, RunResponseType, 500, nil, fmt.Sprintf("failed to locate agent supervisor actor: %s", err))
		return
	}

	askResp, err := api.self.Ask(ctx, agentSuper, startRequestToProto(req))
	if err != nil {
		api.logger.Error("Failed to deploy workload", slog.Any("error", err))
		respondEnvelope(m, RunResponseType, 500, nil, fmt.Sprintf("failed to deploy workload: %s", err))
		return
	}

	protoResp, ok := askResp.(*actorproto.WorkloadStarted)
	if !ok {
		api.logger.Error("Workload listing response from agent supervisor was not the correct type")
		respondEnvelope(m, RunResponseType, 500, nil, "Agent supervisor returned the wrong data type")
		return
	}

	respondEnvelope(m, RunResponseType, 200, startResponseFromProto(protoResp), "")
}

func (api *ControlAPI) handleUndeploy(m *nats.Msg) {
	req := new(nodecontrol.StopWorkloadRequestJson)
	err := json.Unmarshal(m.Data, req)
	if err != nil {
		api.logger.Error("Failed to unmarshal undeploy request", slog.Any("error", err))
		respondEnvelope(m, StopResponseType, 500, nil, fmt.Sprintf("failed to unmarshal undeploy request: %s", err))
		return
	}

	ctx := context.Background()
	_, agentSuper, err := api.self.ActorSystem().ActorOf(ctx, AgentSupervisorActorName)
	if err != nil {
		api.logger.Error("Failed to locate agent supervisor actor", slog.Any("error", err))
		respondEnvelope(m, StopResponseType, 500, nil, fmt.Sprintf("failed to locate agent supervisor actor: %s", err))
		return
	}

	askResp, err := api.self.Ask(ctx, agentSuper, stopRequestToProto(req))
	if err != nil {
		api.logger.Error("Failed to undeploy workload", slog.Any("error", err))
		respondEnvelope(m, StopResponseType, 500, nil, fmt.Sprintf("failed to undeploy workload: %s", err))
		return
	}

	protoResp, ok := askResp.(*actorproto.WorkloadStopped)
	if !ok {
		api.logger.Error("Workload listing response from agent supervisor was not the correct type")
		respondEnvelope(m, StopResponseType, 500, nil, "Agent supervisor returned the wrong data type")
		return
	}

	respondEnvelope(m, StopResponseType, 200, stopResponseFromProto(protoResp), "")
}

func (api *ControlAPI) handleInfo(m *nats.Msg) {
	ctx := context.Background()
	_, agentSuper, err := api.self.ActorSystem().ActorOf(ctx, AgentSupervisorActorName)
	if err != nil {
		api.logger.Error("Failed to locate agent supervisor actor", slog.Any("error", err))
		respondEnvelope(m, InfoResponseType, 500, nil, fmt.Sprintf("failed to locate agent supervisor actor: %s", err))
		return
	}
	// Ask the agent supervisor for a list of all the workloads from all of its children
	response, err := api.self.Ask(ctx, agentSuper, new(actorproto.GetNodeInfo))
	if err != nil {
		api.logger.Error("Failed to get node info", slog.Any("error", err))
		respondEnvelope(m, InfoResponseType, 500, nil, fmt.Sprintf("failed to get node info: %s", err))
		return
	}

	workloadResponse, ok := response.(*actorproto.NodeInfo)
	if !ok {
		api.logger.Error("Workload listing response from agent supervisor was not the correct type")
		respondEnvelope(m, InfoResponseType, 500, nil, "Agent supervisor returned the wrong data type")
		return
	}

	respondEnvelope(m, InfoResponseType, 200, infoResponseFromProto(workloadResponse), "")
}

func (api *ControlAPI) handleLameDuck(m *nats.Msg) {
	ctx := context.Background()
	_, agentSuper, err := api.self.ActorSystem().ActorOf(ctx, AgentSupervisorActorName)
	if err != nil {
		api.logger.Error("Failed to locate agent supervisor actor", slog.Any("error", err))
		respondEnvelope(m, LameDuckResponseType, 500, nil, fmt.Sprintf("failed to locate agent supervisor actor: %s", err))
		return
	}
	// Ask the agent supervisor for a list of all the workloads from all of its children
	response, err := api.self.Ask(ctx, agentSuper, new(actorproto.SetLameDuck))
	if err != nil {
		api.logger.Error("Failed to put node in lame duck mode", slog.Any("error", err))
		respondEnvelope(m, LameDuckResponseType, 500, nil, fmt.Sprintf("failed to put node in lame duck mode: %s", err))
		return
	}

	workloadResponse, ok := response.(*actorproto.LameDuckResponse)
	if !ok {
		api.logger.Error("Workload listing response from agent supervisor was not the correct type")
		respondEnvelope(m, LameDuckResponseType, 500, nil, "Agent supervisor returned the wrong data type")
		return
	}

	respondEnvelope(m, LameDuckResponseType, 200, lameDuckResponseFromProto(workloadResponse), "")
}

func (api *ControlAPI) handlePing(m *nats.Msg) {
	ctx := context.Background()
	_, agentSuper, err := api.self.ActorSystem().ActorOf(ctx, AgentSupervisorActorName)
	if err != nil {
		api.logger.Error("Failed to locate agent supervisor actor", slog.Any("error", err))
		respondEnvelope(m, LameDuckResponseType, 500, nil, fmt.Sprintf("failed to locate agent supervisor actor: %s", err))
		return
	}

	response, err := api.self.Ask(ctx, agentSuper, new(actorproto.PingNode))
	if err != nil {
		api.logger.Error("Failed to ping node", slog.Any("error", err))
		respondEnvelope(m, LameDuckResponseType, 500, nil, fmt.Sprintf("failed to ping node: %s", err))
		return
	}

	workloadResponse, ok := response.(*actorproto.PingNodeResponse)
	if !ok {
		api.logger.Error("Workload listing response from agent supervisor was not the correct type")
		respondEnvelope(m, LameDuckResponseType, 500, nil, "Agent supervisor returned the wrong data type")
		return
	}

	respondEnvelope(m, PingResponseType, 200, pingResponseFromProto(workloadResponse), "")
}

func (api *ControlAPI) handleAgentPing(m *nats.Msg) {
	req := new(nodecontrol.AgentPingResponseJson)
	err := json.Unmarshal(m.Data, req)
	if err != nil {
		api.logger.Error("Failed to unmarshal agent ping request", slog.Any("error", err))
		respondEnvelope(m, AgentPingResponseType, 500, nil, fmt.Sprintf("failed to unmarshal agent ping request: %s", err))
		return
	}

	ctx := context.Background()
	_, agentSuper, err := api.self.ActorSystem().ActorOf(ctx, AgentSupervisorActorName)
	if err != nil {
		api.logger.Error("Failed to locate agent supervisor actor", slog.Any("error", err))
		respondEnvelope(m, AgentPingResponseType, 500, nil, fmt.Sprintf("failed to locate agent supervisor actor: %s", err))
		return
	}

	response, err := api.self.Ask(ctx, agentSuper, new(actorproto.PingAgent))
	if err != nil {
		api.logger.Error("Failed to ping agent", slog.Any("error", err))
		respondEnvelope(m, AgentPingResponseType, 500, nil, fmt.Sprintf("failed to ping agent: %s", err))
		return
	}

	workloadResponse, ok := response.(*actorproto.PingAgentResponse)
	if !ok {
		api.logger.Error("Workload listing response from agent supervisor was not the correct type")
		respondEnvelope(m, AgentPingResponseType, 500, nil, "Agent supervisor returned the wrong data type")
		return
	}

	respondEnvelope(m, AgentPingResponseType, 200, agentPingResponseFromProto(workloadResponse), "")
}

type Envelope struct {
	PayloadType string      `json:"type"`
	Data        interface{} `json:"data,omitempty"`
	Error       *string     `json:"error,omitempty"`
	Code        int         `json:"code"`
}

func newEnvelope(dataType string, data interface{}, code int, err *string) Envelope {
	return Envelope{
		PayloadType: dataType,
		Data:        data,
		Error:       err,
	}
}

func respondEnvelope(m *nats.Msg, dataType string, code int, data interface{}, err string) {
	e := &err
	if len(strings.TrimSpace(err)) == 0 {
		e = nil
	}
	bytes, _ := json.Marshal(newEnvelope(dataType, data, code, e))
	_ = m.Respond(bytes)
}
