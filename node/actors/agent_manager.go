package actors

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/synadia-io/nex/node/options"
)

func createAgentManager() gen.ProcessBehavior {
	return &agentManager{}
}

// Agent manager is responsible for starting one agent per workload type, supplying it with
// the right parameters, and keeping track of the information received from its initial
// registration
type agentManager struct {
	act.Actor

	nodeOptions options.NodeOptions
}

func (mgr *agentManager) Init(args ...any) error {
	mgr.nodeOptions = args[0].(options.NodeOptions)
	mgr.Send(mgr.PID(), "post_init")
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

func (mgr *agentManager) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "post_init":
		// making subscription using MonitorEvent of the gen.Process interface
		if _, err := mgr.MonitorEvent(InternalNatsServerReady); err != nil {
			return err
		}
		mgr.Log().Info("successfully subscribed to: %s", InternalNatsServerReady)
	}
	return nil
}

func (mgr *agentManager) HandleEvent(event gen.MessageEvent) error {
	mgr.Log().Info("received event %s", event.Event)

	switch event.Event.Name {
	case InternalNatsServerReadyName:
		return mgr.startWorkloadAgents(event.Message.(InternalNatsServerReadyEvent))
	}
	return nil
}

func (mgr *agentManager) startWorkloadAgents(evt InternalNatsServerReadyEvent) error {
	mgr.Log().Info("starting %d agent binaries..", len(evt.AgentCredentials))
	// TODO:
	// start all the agent binaries for supported workload types
	return nil
}
