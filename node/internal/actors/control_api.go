package actors

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"disorder.dev/shandler"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/splode/fname"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
	"google.golang.org/protobuf/reflect/protoreflect"

	nodecontrol "github.com/synadia-io/nex/api/nodecontrol/gen"
	"github.com/synadia-io/nex/models"
	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
)

const ControlAPIActorName = "control_api"
const DefaultAskDuration = 10 * time.Second

const (
	AuctionResponseType       = "io.nats.nex.v2.auction_response"
	InfoResponseType          = "io.nats.nex.v2.info_response"
	PingResponseType          = "io.nats.nex.v2.ping_response"
	AgentPingResponseType     = "io.nats.nex.v2.agent_ping_response"
	WorkloadPingResponseType  = "io.nats.nex.v2.workload_ping_response"
	NamespacePingResponseType = "io.nats.nex.v2.namespace_ping_response"
	RunResponseType           = "io.nats.nex.v2.run_response"
	StopResponseType          = "io.nats.nex.v2.stop_response"
	LameDuckResponseType      = "io.nats.nex.v2.lameduck_response"
	CloneWorkloadResponseType = "io.nats.nex.v2.clone_workload_response"
)

type ControlAPI struct {
	nc         *nats.Conn
	logger     *slog.Logger
	publicKey  string
	publicXKey string
	subsz      []*nats.Subscription

	nodeCallback ControlAPINodeCallback

	self *goakt.PID
}

type ControlAPINodeCallback interface {
	Auction(string, []string, map[string]string) (*actorproto.AuctionResponse, error)
	Ping() (*actorproto.PingNodeResponse, error)
	GetInfo(string) (*actorproto.NodeInfo, error)
	SetLameDuck(context.Context)
	IsTargetNode(string) (bool, nkeys.KeyPair, error)
	EncryptPayload([]byte, string) ([]byte, string, error)
	DecryptPayload([]byte) ([]byte, error)
	EmitEvent(string, cloudevents.Event) error
}

func CreateControlAPI(nc *nats.Conn, logger *slog.Logger, publicKey string, callback ControlAPINodeCallback) *ControlAPI {
	api := &ControlAPI{
		nc:           nc,
		logger:       logger,
		publicKey:    publicKey,
		subsz:        make([]*nats.Subscription, 0),
		nodeCallback: callback,
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
		err := sub.Drain()
		if err != nil {
			a.logger.Error("Failed to drain API subscription", slog.String("subscription", sub.Subject))
		}
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
			return
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

	subscriptions := []struct {
		Subject string
		Handler nats.MsgHandler
	}{
		{AuctionSubscribeSubject(), api.handleAuction},
		{UndeploySubscribeSubject(), api.handleUndeploy},
		{AuctionDeploySubscribeSubject(), api.handleADeploy},
		{CloneWorkloadSubscribeSubject(), api.handleCloneWorkload},
		{NamespacePingSubscribeSubject(), api.handleNamespacePing},
		{WorkloadPingSubscribeSubject(), api.handleWorkloadPing},
		// System only subscriptions
		{models.PingSubject(), api.handlePing},
		{models.DirectDeploySubject(api.publicKey), api.handleDeploy},
		{models.LameduckSubject(api.publicKey), api.handleLameDuck},
		{models.DirectPingSubject(api.publicKey), api.handlePing},
		{models.InfoSubject(api.publicKey), api.handleInfo},
	}

	for _, s := range subscriptions {
		sub, err = api.nc.Subscribe(s.Subject, s.Handler)
		if err != nil {
			api.logger.Error("Failed to subscribe to "+s.Subject, slog.Any("error", err), slog.String("id", api.publicKey))
			return err
		}
		api.subsz = append(api.subsz, sub)
	}

	api.logger.Info("NATS execution engine awaiting commands")
	return nil
}

func (api *ControlAPI) handleCloneWorkload(m *nats.Msg) {
	// $NEX.control.namespace.CLONE.workloadid
	sSub := strings.SplitN(m.Subject, ".", 5)
	namespace := sSub[2]
	workloadId := sSub[4]

	req := new(nodecontrol.CloneWorkloadRequestJson)
	err := json.Unmarshal(m.Data, req)
	if err != nil {
		api.logger.Error("Failed to unmarshal clone workload request", slog.Any("error", err))
		models.RespondEnvelope(m, CloneWorkloadResponseType, 500, "", fmt.Sprintf("failed to unmarshal clone workload request: %s", err))
		return
	}

	ctx := context.Background()
	_, agentSuper, err := api.self.ActorSystem().ActorOf(ctx, AgentSupervisorActorName)
	if err != nil {
		api.logger.Error("Failed to locate agent supervisor actor", slog.Any("error", err))
		models.RespondEnvelope(m, CloneWorkloadResponseType, 500, "", fmt.Sprintf("failed to locate agent supervisor actor: %s", err))
		return
	}

	var startRequest *actorproto.StartWorkload
	for _, child := range agentSuper.Children() {
		resp, err := child.Ask(ctx, child, &actorproto.PingWorkload{
			Namespace:  namespace,
			WorkloadId: workloadId,
		}, DefaultAskDuration)
		if err != nil {
			continue
		}
		_, ok := resp.(*actorproto.PingWorkloadResponse)
		if ok {
			rr, err := child.Ask(ctx, child, &actorproto.GetRunRequest{
				Namespace:  namespace,
				WorkloadId: workloadId,
			}, DefaultAskDuration)
			if err != nil {
				api.logger.Error("Failed to get original run request", slog.Any("error", err))
				return
			}
			rwl, ok := rr.(*actorproto.StartWorkload)
			if !ok {
				api.logger.Error("Failed to cast run request to start workload")
				return
			}
			startRequest = rwl
			break
		}
	}
	if startRequest == nil {
		return
	}

	encEnv, err := base64.StdEncoding.DecodeString(startRequest.Environment.Base64EncryptedEnv)
	if err != nil {
		api.logger.Error("Failed to decode base64 env", slog.Any("error", err))
		models.RespondEnvelope(m, CloneWorkloadResponseType, 500, "", fmt.Sprintf("failed to decode base64 env: %s", err))
		return
	}

	clearEnv, err := api.nodeCallback.DecryptPayload(encEnv)
	if err != nil {
		api.logger.Error("Failed to decrypt env", slog.Any("error", err))
		models.RespondEnvelope(m, CloneWorkloadResponseType, 500, "", fmt.Sprintf("failed to decrypt env: %s", err))
		return
	}

	newEncEnv, encBy, err := api.nodeCallback.EncryptPayload(clearEnv, req.NewTargetXkey)
	if err != nil {
		api.logger.Error("Failed to encrypt env", slog.Any("error", err))
		models.RespondEnvelope(m, CloneWorkloadResponseType, 500, "", fmt.Sprintf("failed to encrypt env: %s", err))
		return
	}

	b64EncEnv := base64.StdEncoding.EncodeToString(newEncEnv)
	startRequest.Environment = &actorproto.EncEnvironment{
		Base64EncryptedEnv: b64EncEnv,
		EncryptedBy:        encBy,
	}

	ret := new(nodecontrol.CloneWorkloadResponseJson)
	ret.StartWorkloadRequest = startRequestFromProto(startRequest)
	models.RespondEnvelope(m, AuctionResponseType, 200, ret, "")
}

func (api *ControlAPI) handleAuction(m *nats.Msg) {
	req := new(nodecontrol.AuctionRequestJson)
	err := json.Unmarshal(m.Data, req)
	if err != nil {
		api.logger.Log(context.Background(), shandler.LevelTrace, "Failed to unmarshal auction request", slog.Any("error", err))
		models.RespondEnvelope(m, AuctionResponseType, 500, "", fmt.Sprintf("failed to unmarshal auction request: %s", err))
		return
	}

	convertedAgentType := make([]string, len(req.AgentType))
	for _, at := range req.AgentType {
		convertedAgentType = append(convertedAgentType, string(at))
	}

	auctResp, err := api.nodeCallback.Auction(req.AuctionId, convertedAgentType, req.Tags.Tags)
	if err != nil {
		api.logger.Log(context.Background(), shandler.LevelTrace, "Failed to generate auction response", slog.Any("error", err))
		return
	}

	if auctResp == nil {
		api.logger.Log(context.Background(), shandler.LevelTrace, "No auction response generated")
		return
	}

	models.RespondEnvelope(m, AuctionResponseType, 200, auctionResponseFromProto(auctResp), "")
}

func (api *ControlAPI) handleADeploy(m *nats.Msg) {
	// $NEX.control.default.ADEPLOY.bidderId
	splitSub := strings.SplitN(m.Subject, ".", 5)

	req := new(nodecontrol.StartWorkloadRequestJson)
	err := json.Unmarshal(m.Data, req)
	if err != nil {
		api.logger.Error("Failed to unmarshal deploy request", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to unmarshal deploy request: %s", err))
		return
	}

	if req.WorkloadName == "" {
		rng := fname.NewGenerator()
		req.WorkloadName, err = rng.Generate()
		if err != nil {
			req.WorkloadName = "unnamed-workload"
		}
	}

	// splitSub[4] is the bidderId
	target, xkp, err := api.nodeCallback.IsTargetNode(splitSub[4])
	if err != nil {
		api.logger.Error("Failed to check if target node", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to check if target node: %s", err))
		return
	}

	if !target {
		return
	}

	// reencrypt env with node key
	encEnv, err := base64.StdEncoding.DecodeString(req.EncEnvironment.Base64EncryptedEnv)
	if err != nil {
		api.logger.Error("Failed to decode base64 env", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to decode base64 env: %s", err))
		return
	}

	env, err := xkp.Open(encEnv, req.EncEnvironment.EncryptedBy)
	if err != nil {
		api.logger.Error("Failed to decrypt env", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to decrypt env: %s", err))
		return
	}

	newEncEnv, encTo, err := api.nodeCallback.EncryptPayload(env, "")
	if err != nil {
		api.logger.Error("Failed to encrypt env", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to encrypt env: %s", err))
		return
	}

	req.EncEnvironment = nodecontrol.SharedEncEnvJson{
		Base64EncryptedEnv: base64.StdEncoding.EncodeToString(newEncEnv),
		EncryptedBy:        encTo,
	}

	ctx := context.Background()
	_, agent, err := api.self.ActorSystem().ActorOf(ctx, req.WorkloadType)
	if err != nil {
		api.logger.Error("Failed to locate agent actor", slog.String("type", req.WorkloadType), slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to locate [%s] agent actor: %s", req.WorkloadType, err))
		return
	}

	askResp, err := api.self.Ask(ctx, agent, startRequestToProto(req), DefaultAskDuration)
	if err != nil {
		api.logger.Error("Failed to start workload", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("Failed to start workload: %s", err))
		return
	}

	protoResp, ok := askResp.(*actorproto.Envelope)
	if !ok {
		api.logger.Error("Start workload response from agent was not the correct type")
		models.RespondEnvelope(m, RunResponseType, 500, "", "Agent returned the wrong data type")
		return
	}

	if protoResp.Error != nil {
		api.logger.Error("Agent returned an error", slog.Any("error", protoResp.Error))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("agent returned an error: %s", protoResp.Error))
		return
	}

	var workloadStarted actorproto.WorkloadStarted
	err = protoResp.Payload.UnmarshalTo(&workloadStarted)
	if err != nil {
		api.logger.Error("Failed to unmarshal workload started response", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to unmarshal workload started response: %s", err))
		return
	}

	models.RespondEnvelope(m, RunResponseType, 200, startResponseFromProto(&workloadStarted), "")
}

func (api *ControlAPI) handleDeploy(m *nats.Msg) {
	req := new(nodecontrol.StartWorkloadRequestJson)
	err := json.Unmarshal(m.Data, req)
	if err != nil {
		api.logger.Error("Failed to unmarshal deploy request", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to unmarshal deploy request: %s", err))
		return
	}

	if req.WorkloadName == "" {
		rng := fname.NewGenerator()
		req.WorkloadName, err = rng.Generate()
		if err != nil {
			req.WorkloadName = "unnamed-workload"
		}
	}

	ctx := context.Background()
	_, agent, err := api.self.ActorSystem().ActorOf(ctx, req.WorkloadType)
	if err != nil {
		api.logger.Error("Failed to locate agent actor", slog.String("type", req.WorkloadType), slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to locate [%s] agent actor: %s", req.WorkloadType, err))
		return
	}

	askResp, err := api.self.Ask(ctx, agent, startRequestToProto(req), DefaultAskDuration)
	if err != nil {
		api.logger.Error("Failed to start workload", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("Failed to start workload: %s", err))
		return
	}

	protoResp, ok := askResp.(*actorproto.Envelope)
	if !ok {
		api.logger.Error("Start workload response from agent was not the correct type")
		models.RespondEnvelope(m, RunResponseType, 500, "", "Agent returned the wrong data type")
		return
	}

	if protoResp.Error != nil {
		api.logger.Error("Agent returned an error", slog.Any("error", protoResp.Error))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("agent returned an error: %s", protoResp.Error))
		return
	}

	var workloadStarted actorproto.WorkloadStarted
	err = protoResp.Payload.UnmarshalTo(&workloadStarted)
	if err != nil {
		api.logger.Error("Failed to unmarshal workload started response", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to unmarshal workload started response: %s", err))
		return
	}

	models.RespondEnvelope(m, RunResponseType, 200, startResponseFromProto(&workloadStarted), "")
}

func (api *ControlAPI) handleUndeploy(m *nats.Msg) {
	// $NEX.control.namespace.UNDEPLOY.workloadid
	splitSub := strings.SplitN(m.Subject, ".", 5)
	namespace := splitSub[2]
	workloadId := splitSub[4]

	var err error
	var askResp protoreflect.ProtoMessage

	_, agentSuper, err := api.self.ActorSystem().ActorOf(context.Background(), AgentSupervisorActorName)
	if err != nil {
		api.logger.Error("Failed to locate agent supervisor actor", slog.Any("error", err))
		return
	}

findWorkload:
	for _, child := range agentSuper.Children() { // iterate over all agents
		for _, grandchild := range child.Children() { // iterate over all workloads
			if grandchild.Name() == workloadId {
				askResp, err = api.self.Ask(context.Background(), child, &actorproto.StopWorkload{Namespace: namespace, WorkloadId: workloadId}, DefaultAskDuration)
				if err != nil {
					api.logger.Error("Failed to stop workload", slog.Any("error", err))
					models.RespondEnvelope(m, StopResponseType, 500, "", fmt.Sprintf("Failed to stop workload: %s", err))
					return
				}
				break findWorkload
			}
		}
	}

	if askResp == nil {
		return // this node does not have the workload
	}

	protoResp, ok := askResp.(*actorproto.Envelope)
	if !ok {
		api.logger.Error("Workload stop response from agent was not the correct type")
		models.RespondEnvelope(m, StopResponseType, 500, "", "Agent returned the wrong data type for workload stop")
		return
	}

	if protoResp.Error != nil {
		api.logger.Error("Agent returned an error", slog.Any("error", protoResp.Error))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("agent returned an error: %s", protoResp.Error))
		return
	}

	var workloadStopped actorproto.WorkloadStopped
	err = protoResp.Payload.UnmarshalTo(&workloadStopped)
	if err != nil {
		api.logger.Error("Failed to unmarshal workload started response", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to unmarshal workload started response: %s", err))
		return
	}

	models.RespondEnvelope(m, StopResponseType, 200, stopResponseFromProto(&workloadStopped), "")
}

func (api *ControlAPI) handleInfo(m *nats.Msg) {
	req := new(nodecontrol.NodeInfoRequestJson)
	err := json.Unmarshal(m.Data, req)
	if err != nil {
		api.logger.Error("Failed to unmarshal info request", slog.Any("error", err))
		models.RespondEnvelope(m, InfoResponseType, 500, "", fmt.Sprintf("failed to unmarshal info request: %s", err))
		return
	}

	info, err := api.nodeCallback.GetInfo(req.Namespace)
	if err != nil {
		api.logger.Error("Failed to get node info", slog.Any("error", err))
		models.RespondEnvelope(m, InfoResponseType, 500, "", fmt.Sprintf("failed to get node info: %s", err))
		return
	}

	models.RespondEnvelope(m, InfoResponseType, 200, infoResponseFromProto(info), "")
}

func (api *ControlAPI) handleLameDuck(m *nats.Msg) {
	api.logger.Debug("Received lame duck request")

	req := new(nodecontrol.LameduckRequestJson)
	err := json.Unmarshal(m.Data, req)
	if err != nil {
		api.logger.Error("Failed to unmarshal lame duck request", slog.Any("error", err))
		models.RespondEnvelope(m, LameDuckResponseType, 500, "", fmt.Sprintf("failed to unmarshal lame duck request: %s", err))
		return
	}

	delay, err := time.ParseDuration(req.Delay)
	if err != nil {
		api.logger.Error("Failed to parse lame duck delay", slog.Any("error", err))
		models.RespondEnvelope(m, LameDuckResponseType, 500, "", fmt.Sprintf("failed to parse lame duck delay: %s", err))
		return
	}

	// adds 30 seconds to the delay to allow workloads to shutdown
	ctx, cancel := context.WithTimeout(context.Background(), delay+30*time.Second)
	api.nodeCallback.SetLameDuck(ctx)
	go func() {
		api.logger.Warn("Lameduck mode enabled. Will start shutting down workloads.", slog.String("workload_shutdown", time.Now().Add(delay).Format(time.DateTime)))
		time.Sleep(delay)
		_, agentSuper, err := api.self.ActorSystem().ActorOf(ctx, AgentSupervisorActorName)
		if err != nil {
			api.logger.Error("Failed to locate agent supervisor actor", slog.Any("error", err))
			return
		}

		err = api.self.Tell(ctx, agentSuper, &actorproto.SetLameDuck{})
		if err != nil {
			api.logger.Error("Failed to put node in lame duck mode", slog.Any("error", err))
			return
		}

		ticker := time.NewTicker(100 * time.Millisecond)
		for _ = range ticker.C {
			if agentSuper.ChildrenCount() == 0 {
				ticker.Stop()
				cancel()
				break
			}
		}
	}()

	models.RespondEnvelope(m, LameDuckResponseType, 200, &nodecontrol.LameduckResponseJson{Success: true}, "")
}

func (api *ControlAPI) handlePing(m *nats.Msg) {
	pingResponse, err := api.nodeCallback.Ping()
	if err != nil {
		api.logger.Error("failed to ping node", slog.Any("error", err))
		models.RespondEnvelope(m, PingResponseType, 500, "", fmt.Sprintf("failed to ping node: %s", err.Error()))
		return
	}

	models.RespondEnvelope(m, PingResponseType, 200, pingResponseFromProto(pingResponse), "")
}

func (api *ControlAPI) handleWorkloadPing(m *nats.Msg) {
	ctx := context.Background()

	splitSub := strings.SplitN(m.Subject, ".", 6)
	var workloadType, namespace, workloadID string

	// PREFIX.control.<NAMESPACE>.WPING.<WORKLOAD_TYPE>.<WORKLOAD_ID>
	namespace = splitSub[2]
	workloadType = splitSub[4]
	workloadID = splitSub[5]

	_, agent, err := api.self.ActorSystem().ActorOf(ctx, workloadType)
	if err != nil {
		return
	}

	response, err := api.self.Ask(ctx, agent, &actorproto.PingWorkload{
		Type:       workloadType,
		Namespace:  namespace,
		WorkloadId: workloadID,
	}, DefaultAskDuration)
	if err != nil {
		return
	}

	aWorkloadPingResponse, ok := response.(*actorproto.PingWorkloadResponse)
	if !ok {
		return
	}

	models.RespondEnvelope(m, WorkloadPingResponseType, 200, workloadPingResponseFromProto(aWorkloadPingResponse), "")
}

func (api *ControlAPI) handleNamespacePing(m *nats.Msg) {
	// #NEX.control.<NAMESPACE>.WPING
	ctx := context.Background()

	splitSub := strings.SplitN(m.Subject, ".", 4)

	namespace := splitSub[2]

	_, supervisor, err := api.self.ActorSystem().ActorOf(ctx, AgentSupervisorActorName)
	if err != nil {
		api.logger.Error("Failed to locate agent supervisor actor", slog.Any("error", err))
		return
	}

	var workloads []nodecontrol.WorkloadSummary
	for _, agent := range supervisor.Children() {
		respEnv, err := api.self.Ask(ctx, agent, &actorproto.QueryWorkloads{}, DefaultAskDuration)
		if err != nil {
			continue
		}
		resp, ok := respEnv.(*actorproto.Envelope)
		if !ok {
			api.logger.Error("Failed to cast response to envelope", slog.Any("error", err))
			continue
		}

		var workloadResp actorproto.WorkloadList
		err = resp.Payload.UnmarshalTo(&workloadResp)
		if err != nil {
			api.logger.Error("Failed to unmarshal workload list response", slog.Any("error", err))
			continue
		}

		for _, workload := range workloadResp.Workloads {
			if namespace == "system" || workload.Namespace == namespace {
				workloads = append(workloads, nodecontrol.WorkloadSummary{
					Id:            workload.Id,
					Name:          workload.Name,
					Runtime:       workload.Runtime,
					StartTime:     workload.StartedAt.AsTime().Format(time.DateTime),
					WorkloadState: workload.State,
					WorkloadType:  workload.WorkloadType,
				})
			}
		}
	}

	models.RespondEnvelope(m, NamespacePingResponseType, 200, workloads, "")
}
