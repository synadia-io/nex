package actors

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func createAgentManager() gen.ProcessBehavior {
	return &agentManager{}
}

// Agent manager is responsible for starting one agent per workload type, supplying it with
// the right parameters, and keeping track of the information received from its initial
// registration
type agentManager struct {
	act.Actor
}

func (mgr *agentManager) Init(args ...any) error {
	return nil
}

// at some point there will be some HandleCall handlers in here where other processes
// do process.Call("agent_manager", ...) in order to query information about running
// agents, etc.

// HandleInspect invoked on the request made with gen.Process.Inspect(...)
func (mgr *agentManager) HandleInspect(from gen.PID, item ...string) map[string]string {
	mgr.Log().Info("agent manager got inspect request from %s", from)
	return nil
}
