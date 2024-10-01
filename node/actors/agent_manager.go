package actors

import (
	"errors"
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

type agentManagerParams struct {
	options models.NodeOptions
}

func (p *agentManagerParams) Validate() error {
	var err error

	// insert validations
	// validate options much?

	return err
}

func (mgr *agentManager) Init(args ...any) error {
	if len(args) != 1 {
		err := errors.New("agent manager params are required")
		mgr.Log().Error("Failed to start agent manager", slog.String("error", err.Error()))
		return err
	}

	if _, ok := args[0].(agentManagerParams); !ok {
		err := errors.New("args[0] must be valid agent manager params")
		mgr.Log().Error("Failed to start agent manager", slog.String("error", err.Error()))
		return err
	}

	params := args[0].(agentManagerParams)
	err := params.Validate()
	if err != nil {
		mgr.Log().Error("Failed to start agent manager", slog.String("error", err.Error()))
		return err
	}

	err = mgr.Send(mgr.PID(), PostInit)
	if err != nil {
		return err
	}

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
	mgr.Log().Info("received event", slog.String("event_name", string(event.Event.Name)))

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
