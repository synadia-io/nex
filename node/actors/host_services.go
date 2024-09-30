package actors

import (
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
	hostServicesOptions := args[0].(models.HostServiceOptions)

	hs.Log().Info("Host services started", slog.String("nats_url", hostServicesOptions.NatsUrl))

	return nil
}

// HandleInspect invoked on the request made with gen.Process.Inspect(...)
func (hs *hostServicesServer) HandleInspect(from gen.PID, item ...string) map[string]string {
	hs.Log().Info("host services server got inspect request from %s", from)
	return nil
}
