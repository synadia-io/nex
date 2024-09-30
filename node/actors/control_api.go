package actors

import (
	"errors"
	"log/slog"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

const APIPrefix = "$NEX"

func createControlAPI() gen.ProcessBehavior {
	return &controlAPI{
		subsz: make([]*nats.Subscription, 0),
	}
}

type controlAPI struct {
	act.Actor

	publicKey  string
	publicXKey string
	subsz      []*nats.Subscription
}

func (api *controlAPI) Init(args ...any) error {
	if len(args) != 2 {
		err := errors.New("NATS connection and public key are required")
		api.Log().Error("Failed to start control API", slog.String("error", err.Error()))
		return err
	}

	if _, ok := args[0].(*nats.Conn); !ok {
		err := errors.New("args[0] must be a valid NATS connection")
		api.Log().Error("Failed to start control API", slog.String("error", err.Error()))
		return err
	}

	if publicKey, ok := args[1].(string); ok {
		api.publicKey = publicKey
	}

	err := api.initPublicXKey()
	if err != nil {
		return err
	}

	err = api.subscribe(args[0].(*nats.Conn))
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
		api.Log().Error("Failed to create x509 curve key", slog.Any("error", err))
		return nil
	}

	publicXKey, err := kp.PublicKey()
	if err != nil {
		api.Log().Error("Failed to get public key from x509 curve key", slog.Any("error", err))
		return nil
	}

	api.publicXKey = publicXKey
	return nil
}

func (api *controlAPI) shutdown() error {
	for _, sub := range api.subsz {
		if !sub.IsValid() {
			continue
		}

		err := sub.Drain()
		if err != nil {
			api.Log().Warning("Failed to drain API subscription", slog.String("subject", sub.Subject))
		}
	}

	return nil
}

func (api *controlAPI) subscribe(nc *nats.Conn) error {
	var sub *nats.Subscription
	var err error

	sub, err = nc.Subscribe(APIPrefix+".AUCTION", api.handleAuction)
	if err != nil {
		api.Log().Error("Failed to subscribe to auction subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = nc.Subscribe(APIPrefix+".DEPLOY.*."+api.publicKey, api.handleDeploy)
	if err != nil {
		api.Log().Error("Failed to subscribe to run subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = nc.Subscribe(APIPrefix+".INFO.*."+api.publicKey, api.handleInfo)
	if err != nil {
		api.Log().Error("Failed to subscribe to info subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = nc.Subscribe(APIPrefix+".LAMEDUCK."+api.publicKey, api.handleLameDuck)
	if err != nil {
		api.Log().Error("Failed to subscribe to lame duck subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = nc.Subscribe(APIPrefix+".PING", api.handlePing)
	if err != nil {
		api.Log().Error("Failed to subscribe to ping subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = nc.Subscribe(APIPrefix+".PING."+api.publicKey, api.handlePing)
	if err != nil {
		api.Log().Error("Failed to subscribe to node-specific ping subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = nc.Subscribe(APIPrefix+".UNDEPLOY.*."+api.publicKey, api.handleUndeploy)
	if err != nil {
		api.Log().Error("Failed to subscribe to undeploy subject", slog.Any("error", err), slog.String("id", api.publicKey))
		return err
	}
	api.subsz = append(api.subsz, sub)

	sub, err = nc.Subscribe(APIPrefix+".WPING.>", api.handleWorkloadPing)
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
