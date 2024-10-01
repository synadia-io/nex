package actors

import (
	"errors"
	"fmt"
	"log/slog"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

const APIPrefix = "$NEX"

func createControlAPI() gen.ProcessBehavior {
	return &controlAPI{}
}

type controlAPI struct {
	act.Actor

	nc         *nats.Conn
	publicKey  string
	publicXKey string
	subsz      []*nats.Subscription
}

type controlAPIParams struct {
	nc        *nats.Conn
	publicKey string
}

func (p *controlAPIParams) Validate() error {
	var err error

	if p.nc == nil {
		err = errors.Join(err, errors.New("valid NATS connection is required"))
	}

	if p.publicKey == "" {
		err = errors.Join(err, errors.New("public key is required"))
	}

	return err
}

func (api *controlAPI) Init(args ...any) error {
	if len(args) != 1 {
		err := errors.New("control API params are required")
		api.Log().Error("Failed to start control API", slog.String("error", err.Error()))
		return err
	}

	if _, ok := args[0].(controlAPIParams); !ok {
		err := errors.New("args[0] must be valid control API params")
		api.Log().Error("Failed to start control API", slog.String("error", err.Error()))
		return err
	}

	params := args[0].(controlAPIParams)
	err := params.Validate()
	if err != nil {
		api.Log().Error("Failed to start control API", slog.String("error", err.Error()))
		return err
	}

	api.nc = params.nc
	api.publicKey = params.publicKey

	err = api.initPublicXKey()
	if err != nil {
		return err
	}

	err = api.subscribe()
	if err != nil {
		_ = api.shutdown()
		return err
	}

	api.Log().Info("Control API started",
		slog.String("public_key", api.publicKey),
		slog.String("public_xkey", api.publicXKey),
	)

	return nil
}

// HandleInspect invoked on the request made with gen.Process.Inspect(...)
func (api *controlAPI) HandleInspect(from gen.PID, item ...string) map[string]string {
	api.Log().Info("control API server got inspect request from %s", from)
	return nil
}

func (api *controlAPI) initPublicXKey() error {
	kp, err := nkeys.CreateCurveKeys()
	if err != nil {
		api.Log().Error("Failed to create X25519 keypair", slog.Any("error", err))
		return nil
	}

	publicXKey, err := kp.PublicKey()
	if err != nil {
		api.Log().Error("Failed to get X25519 public key", slog.Any("error", err))
		return nil
	}

	api.publicXKey = publicXKey
	return nil
}

func (api *controlAPI) shutdown() error {
	var err error

	for _, sub := range api.subsz {
		if !sub.IsValid() {
			continue
		}

		_err := sub.Drain()
		if _err != nil {
			api.Log().Warning("Failed to drain API subscription", slog.String("subject", sub.Subject))
			err = errors.Join(_err, fmt.Errorf("failed to drain API subscription: %s", sub.Subject))
		}
	}

	return err
}

func (api *controlAPI) subscribe() error {
	var sub *nats.Subscription
	var err error

	sub, err = api.nc.Subscribe(APIPrefix+".AUCTION", api.handleAuction)
	if err != nil {
		api.Log().Error("Failed to subscribe to auction subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".DEPLOY.*."+api.publicKey, api.handleDeploy)
	if err != nil {
		api.Log().Error("Failed to subscribe to run subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".INFO.*."+api.publicKey, api.handleInfo)
	if err != nil {
		api.Log().Error("Failed to subscribe to info subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".LAMEDUCK."+api.publicKey, api.handleLameDuck)
	if err != nil {
		api.Log().Error("Failed to subscribe to lame duck subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".PING", api.handlePing)
	if err != nil {
		api.Log().Error("Failed to subscribe to ping subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".PING."+api.publicKey, api.handlePing)
	if err != nil {
		api.Log().Error("Failed to subscribe to node-specific ping subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".UNDEPLOY.*."+api.publicKey, api.handleUndeploy)
	if err != nil {
		api.Log().Error("Failed to subscribe to undeploy subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = api.nc.Subscribe(APIPrefix+".WPING.>", api.handleWorkloadPing)
	if err != nil {
		api.Log().Error("Failed to subscribe to workload ping subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	api.Log().Info("NATS execution engine awaiting commands", slog.String("id", api.publicKey)) //slog.String("version", VERSION)
	return nil
}

func (api *controlAPI) handleAuction(m *nats.Msg) {
	// TODO
}

func (api *controlAPI) handleDeploy(m *nats.Msg) {
	// TODO
}

func (api *controlAPI) handleInfo(m *nats.Msg) {
	// TODO
}

func (api *controlAPI) handleLameDuck(m *nats.Msg) {
	// TODO
}

func (api *controlAPI) handlePing(m *nats.Msg) {
	// TODO
}

func (api *controlAPI) handleUndeploy(m *nats.Msg) {
	// TODO
}

func (api *controlAPI) handleWorkloadPing(m *nats.Msg) {
	// TODO
}
