package actors

import (
	"errors"
	"log/slog"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/synadia-io/nex/models"
)

func createHostServices() gen.ProcessBehavior {
	return &hostServicesServer{}
}

type hostServicesServer struct {
	act.Actor
}

func (hs *hostServicesServer) Init(args ...any) error {
	if len(args) != 1 {
		err := errors.New("host service options are required")
		hs.Log().Error("Failed to start host services", slog.String("error", err.Error()))
		return err
	}

	if _, ok := args[0].(models.HostServiceOptions); !ok {
		err := errors.New("args[0] must be valid host service options")
		hs.Log().Error("Failed to start host services", slog.String("error", err.Error()))
		return err
	}

	hostServiceOptions := args[0].(models.HostServiceOptions)

	hs.Log().Info("Host services started", slog.String("nats_url", hostServiceOptions.NatsUrl))

	return nil
}

// HandleInspect invoked on the request made with gen.Process.Inspect(...)
func (hs *hostServicesServer) HandleInspect(from gen.PID, item ...string) map[string]string {
	hs.Log().Info("host services server got inspect request from %s", from)
	return nil
}
