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

type hostServicesServerParams struct {
	options models.HostServiceOptions
}

func (p *hostServicesServerParams) Validate() error {
	var err error

	// insert validations
	// validate options much?

	return err
}

func (hs *hostServicesServer) Init(args ...any) error {
	if len(args) != 1 {
		err := errors.New("host service params are required")
		hs.Log().Error("Failed to start host services", slog.String("error", err.Error()))
		return err
	}

	if _, ok := args[0].(hostServicesServerParams); !ok {
		err := errors.New("args[0] must be valid host service params")
		hs.Log().Error("Failed to start host services", slog.String("error", err.Error()))
		return err
	}

	params := args[0].(hostServicesServerParams)
	err := params.Validate()
	if err != nil {
		hs.Log().Error("Failed to start host services", slog.String("error", err.Error()))
		return err
	}

	hs.Log().Info("Host services started", slog.String("nats_url", params.options.NatsUrl))

	return nil
}

// HandleInspect invoked on the request made with gen.Process.Inspect(...)
func (hs *hostServicesServer) HandleInspect(from gen.PID, item ...string) map[string]string {
	hs.Log().Info("host services server got inspect request from %s", from)
	return nil
}
