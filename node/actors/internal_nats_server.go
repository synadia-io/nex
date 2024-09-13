package actors

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func createInternalNatsServer() gen.ProcessBehavior {
	return &internalNatsServer{}
}

type internalNatsServer struct {
	act.Actor
}

func (ns *internalNatsServer) Init(args ...any) error {
	ns.Log().Info("Internal NATS server started")

	return nil
}

// HandleInspect invoked on the request made with gen.Process.Inspect(...)
func (ns *internalNatsServer) HandleInspect(from gen.PID, item ...string) map[string]string {
	ns.Log().Info("internal nats server got inspect request from %s", from)
	return nil
}
