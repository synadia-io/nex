package actors

import (
	"log/slog"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/synadia-io/nex/models"
)

func createAgentManager() gen.ProcessBehavior {
	return &agentManager{}
}

// Agent manager is responsible for starting one agent per workload type, supplying it with
// the right parameters, and keeping track of the information received from its initial
// registration
type agentManager struct {
	act.Actor

	nodeOptions models.NodeOptions
}

func (mgr *agentManager) Init(args ...any) error {
	mgr.nodeOptions = args[0].(models.NodeOptions)
	_ = mgr.Send(mgr.PID(), PostInit)
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
	case PostInit:
		// making subscription using MonitorEvent of the gen.Process interface
		if _, err := mgr.MonitorEvent(InternalNatsServerReady); err != nil {
			return err
		}
		mgr.Log().Info("successfully subscribed to internal nats server", slog.Any("event_name", InternalNatsServerReady))
	}
	return nil
}

func (mgr *agentManager) HandleEvent(event gen.MessageEvent) error {
	mgr.Log().Info("received event", slog.Any("event", event.Event))

	switch event.Event.Name {
	case InternalNatsServerReadyName:
		return mgr.startWorkloadAgents(event.Message.(InternalNatsServerReadyEvent))
	}
	return nil
}

func (mgr *agentManager) startWorkloadAgents(evt InternalNatsServerReadyEvent) error {
	mgr.Log().Info("starting agent binaries", slog.Int("count", len(evt.AgentCredentials)))
	// TODO:
	// start all the agent binaries for supported workload types
	return nil
}
