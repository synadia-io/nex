package actors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
	"github.com/tochemey/goakt/v2/log"
	"log/slog"

	actorproto "github.com/synadia-io/nex/node/actors/pb"
)

const APIPrefix = "$NEX"

const ControlAPIActorName = "control_api"

func CreateControlAPI(nc *nats.Conn, publicKey string) *ControlAPI {
	api := &ControlAPI{nc: nc, publicKey: publicKey, subsz: make([]*nats.Subscription, 0)}
	err := api.initPublicXKey()
	if err != nil {
		return nil
	}

	return api
}

type ControlAPI struct {
	logger     log.Logger
	nc         *nats.Conn
	publicKey  string
	publicXKey string
	subsz      []*nats.Subscription

	self *goakt.PID
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
		a.logger = ctx.Self().Logger()
		a.self = ctx.Self()
		err := a.subscribe()
		if err != nil {
			_ = a.shutdown()
			ctx.Err(err)
		}
		a.logger.Infof("Control API NATS server '%s' is running", ctx.Self().Name())

	default:
		ctx.Unhandled()
	}
}

func (api *ControlAPI) initPublicXKey() error {
	kp, err := nkeys.CreateCurveKeys()
	if err != nil {
		return err
	}

	publicXKey, err := kp.PublicKey()
	if err != nil {
		return err
	}

	api.publicXKey = publicXKey
	return nil
}

func (api *ControlAPI) shutdown() error {
	var err error

	for _, sub := range api.subsz {
		if !sub.IsValid() {
			continue
		}

		_err := sub.Drain()
		if _err != nil {
			api.logger.Warnf("Failed to drain API subscription: %s", sub.Subject)
			err = errors.Join(_err, fmt.Errorf("failed to drain API subscription: %s", sub.Subject))
		}
	}

	return err
}

func (api *ControlAPI) subscribe() error {
	var sub *nats.Subscription
	var err error

	sub, err = api.nc.Subscribe(APIPrefix+".AUCTION", api.handleAuction)
	if err != nil {
		// TODO: change this to conform to whatever our "real" logging strat is
		api.logger.Error("Failed to subscribe to auction subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".DEPLOY.*."+api.publicKey, api.handleDeploy)
	if err != nil {
		api.logger.Error("Failed to subscribe to run subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".INFO.*."+api.publicKey, api.handleInfo)
	if err != nil {
		api.logger.Error("Failed to subscribe to info subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".LAMEDUCK."+api.publicKey, api.handleLameDuck)
	if err != nil {
		api.logger.Error("Failed to subscribe to lame duck subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".PING", api.handlePing)
	if err != nil {
		api.logger.Error("Failed to subscribe to ping subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".PING."+api.publicKey, api.handlePing)
	if err != nil {
		api.logger.Error("Failed to subscribe to node-specific ping subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".UNDEPLOY.*."+api.publicKey, api.handleUndeploy)
	if err != nil {
		api.logger.Error("Failed to subscribe to undeploy subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".WPING.>", api.handleWorkloadPing)
	if err != nil {
		api.logger.Error("Failed to subscribe to workload ping subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	api.logger.Info("NATS execution engine awaiting commands", slog.String("id", api.publicKey)) //slog.String("version", VERSION)
	return nil
}

func (api *ControlAPI) handleAuction(m *nats.Msg) {
	// TODO
}

func (api *ControlAPI) handleDeploy(m *nats.Msg) {
	// TODO
}

func (api *ControlAPI) handleInfo(m *nats.Msg) {

	req := &actorproto.QueryWorkloads{}
	ctx := context.Background()
	_, agentSuper, err := api.self.ActorSystem().ActorOf(ctx, AgentSupervisorActorName)
	if err != nil {
		api.logger.Error("Failed to locate agent supervisor actor", slog.Any("error", err))
		return
	}
	// Ask the agent supervisor for a list of all the workloads from all of its children
	response, err := api.self.Ask(ctx, agentSuper, req)
	if err != nil {
		api.logger.Error("Failed to get list of running workloads from agent supervisor", slog.Any("error", err))
		return
	}
	workloadResponse, ok := response.(*actorproto.WorkloadListing)
	if !ok {
		api.logger.Error("Workload listing response from agent supervisor was not the correct type")
		return
	}

	// TODO: This struct will be replaced by a generated struct from a JSON schema in api/nodecontrol
	info := struct {
		NodeID    string                        `json:"node_id"`
		Workloads []*actorproto.WorkloadSummary `json:"workloads"`
	}{
		NodeID:    "Nxxx",
		Workloads: workloadResponse.Workloads,
	}

	bytes, err := json.Marshal(&info)
	if err != nil {
		api.logger.Error("Failed to marshal node info to JSON", slog.Any("error", err))
		return
	}

	_ = m.Respond(bytes)
}

func (api *ControlAPI) handleLameDuck(m *nats.Msg) {
	// TODO
}

func (api *ControlAPI) handlePing(m *nats.Msg) {
	// TODO
}

func (api *ControlAPI) handleUndeploy(m *nats.Msg) {
	// TODO
}

func (api *ControlAPI) handleWorkloadPing(m *nats.Msg) {
	// TODO
}
