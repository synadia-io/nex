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
	AuctionResponseType       = "io.nats.nex.v2.auction_response"
	InfoResponseType          = "io.nats.nex.v2.info_response"
	PingResponseType          = "io.nats.nex.v2.ping_response"
	AgentPingResponseType     = "io.nats.nex.v2.agent_ping_response"
	WorkloadPingResponseType  = "io.nats.nex.v2.workload_ping_response"
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
	GetInfo() (*actorproto.NodeInfo, error)
	SetLameDuck(context.Context)
	IsTargetNode(string) (bool, nkeys.KeyPair, error)
	EncryptPayload([]byte, string) ([]byte, string, error)
	DecryptPayload([]byte) ([]byte, error)
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

	sub, err = api.nc.Subscribe(AuctionSubscribeSubject(), api.handleAuction)
	if err != nil {
		api.logger.Error("Failed to subscribe to auction subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(models.DirectDeploySubject(api.publicKey), api.handleDeploy)
	if err != nil {
		api.logger.Error("Failed to subscribe to run subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(AuctionDeploySubscribeSubject(), api.handleADeploy)
	if err != nil {
		api.logger.Error("Failed to subscribe to run subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(InfoSubscribeSubject(), api.handleInfo)
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

	sub, err = api.nc.Subscribe(UndeploySubscribeSubject(), api.handleUndeploy)
	if err != nil {
		api.logger.Error("Failed to subscribe to undeploy subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(WorkloadPingSubscribeSubject(), api.handleWorkloadPing)
	if err != nil {
		api.logger.Error("Failed to subscribe to agent ping subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(CloneWorkloadSubscribeSubject(), api.handleCloneWorkload)
	if err != nil {
		api.logger.Error("Failed to subscribe to clone workload subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

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
		})
		if err != nil {
			continue
		}
		_, ok := resp.(*actorproto.PingWorkloadResponse)
		if ok {
			rr, err := child.Ask(ctx, child, &actorproto.GetRunRequest{
				Namespace:  namespace,
				WorkloadId: workloadId,
			})
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
	// $NEX.control.default.ADEPLOY.OdXiuMFTfXp1njwcArUzD2
	splitSub := strings.SplitN(m.Subject, ".", 5)

	req := new(nodecontrol.StartWorkloadRequestJson)
	err := json.Unmarshal(m.Data, req)
	if err != nil {
		api.logger.Error("Failed to unmarshal deploy request", slog.Any("error", err))
		models.RespondEnvelope(m, RunResponseType, 500, "", fmt.Sprintf("failed to unmarshal deploy request: %s", err))
		return
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

	// TODO: obtain host services credentials for workload (creds service)
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
	if !protoResp.Started {
		api.logger.Warn("Workload not started (agent returned started=false)")
	}

	models.RespondEnvelope(m, RunResponseType, 200, startResponseFromProto(protoResp), "")
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
	info, err := api.nodeCallback.GetInfo()
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
	})
	if err != nil {
		return
	}

	aWorkloadPingResponse, ok := response.(*actorproto.PingWorkloadResponse)
	if !ok {
		return
	}

	models.RespondEnvelope(m, WorkloadPingResponseType, 200, workloadPingResponseFromProto(aWorkloadPingResponse), "")
}
