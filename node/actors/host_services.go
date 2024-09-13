package actors

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func createHostServices() gen.ProcessBehavior {
	return &hostServicesServer{}
}

type hostServicesServer struct {
	act.Actor
}

func (hs *hostServicesServer) Init(args ...any) error {
	hs.Log().Info("Host Services Server started")

	return nil
}

// HandleInspect invoked on the request made with gen.Process.Inspect(...)
func (hs *hostServicesServer) HandleInspect(from gen.PID, item ...string) map[string]string {
	hs.Log().Info("host services server got inspect request from %s", from)
	return nil
}
