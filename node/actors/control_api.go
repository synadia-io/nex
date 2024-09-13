package actors

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func createControlAPIServer() gen.ProcessBehavior {
	return &controlAPIServer{}
}

type controlAPIServer struct {
	act.Actor
}

func (api *controlAPIServer) Init(args ...any) error {
	api.Log().Info("Control API Server started")

	return nil
}

// HandleInspect invoked on the request made with gen.Process.Inspect(...)
func (api *controlAPIServer) HandleInspect(from gen.PID, item ...string) map[string]string {
	api.Log().Info("control API server got inspect request from %s", from)
	return nil
}
