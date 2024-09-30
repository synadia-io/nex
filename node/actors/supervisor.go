package actors

import (
	"errors"
	"log/slog"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/models"
)

const childActorNameAgentManager = "agent_manager"
const childActorNameControlAPI = "control_api"
const childActorNameHostServices = "host_services"
const childActorNameInternalNATSServer = "internal_nats_server"

func createNexSupervisor() gen.ProcessBehavior {
	return &NexSupervisor{}
}

type NexSupervisor struct {
	act.Supervisor
}

// Init invoked on a spawn Supervisor process. This is a mandatory callback for the implementation
func (sup *NexSupervisor) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec

	if len(args) != 2 {
		err := errors.New("NATS connection and node options are required")
		sup.Log().Error("Failed to start nex supervisor", slog.String("error", err.Error()))
		return spec, err
	}

	if _, ok := args[0].(*nats.Conn); !ok {
		err := errors.New("arg[0] must be a valid NATS connection")
		sup.Log().Error("Failed to start nex supervisor", slog.String("error", err.Error()))
		return spec, err
	}

	if _, ok := args[0].(models.NodeOptions); !ok {
		err := errors.New("arg[1] must be valid node options")
		sup.Log().Error("Failed to start nex supervisor", slog.String("error", err.Error()))
		return spec, err
	}

	var nodeID string // FIXME-- this needs to be provided as well...
	nc := args[0].(*nats.Conn)
	nodeOptions := args[1].(models.NodeOptions)

	// set supervisor type
	spec.Type = act.SupervisorTypeOneForOne

	spec.Children = []act.SupervisorChildSpec{
		{
			Name:    childActorNameInternalNATSServer,
			Factory: createInternalNatsServer,
			Args:    []any{nodeOptions},
		},
		{
			Name:    childActorNameHostServices,
			Factory: createHostServices,
			Args:    []any{nodeOptions.HostServiceOptions},
		},
		{
			Name:    childActorNameControlAPI,
			Factory: createControlAPI,
			Args:    []any{nc, nodeID},
		},
		{
			Name:    childActorNameAgentManager,
			Factory: createAgentManager,
			Args:    []any{nodeOptions},
		},
	}

	// set strategy
	spec.Restart.Strategy = act.SupervisorStrategyTransient
	spec.Restart.Intensity = 2 // How big bursts of restarts you want to tolerate.
	spec.Restart.Period = 5    // In seconds.

	return spec, nil
}

//
// Methods below are optional, so you can remove those that aren't be used
//

// HandleChildStart invoked on a successful child process starting if option EnableHandleChild
// was enabled in act.SupervisorSpec
func (sup *NexSupervisor) HandleChildStart(name gen.Atom, pid gen.PID) error {
	return nil
}

// HandleChildTerminate invoked on a child process termination if option EnableHandleChild
// was enabled in act.SupervisorSpec
func (sup *NexSupervisor) HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error {
	return nil
}

// HandleMessage invoked if Supervisor received a message sent with gen.Process.Send(...).
// Non-nil value of the returning error will cause termination of this process.
// To stop this process normally, return gen.TerminateReasonNormal or
// gen.TerminateReasonShutdown. Any other - for abnormal termination.
func (sup *NexSupervisor) HandleMessage(from gen.PID, message any) error {
	sup.Log().Info("supervisor got message from %s", from)
	return nil
}

// HandleCall invoked if Supervisor got a synchronous request made with gen.Process.Call(...).
// Return nil as a result to handle this request asynchronously and
// to provide the result later using the gen.Process.SendResponse(...) method.
func (sup *NexSupervisor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	sup.Log().Info("supervisor got request from %s with reference %s", from, ref)
	return gen.Atom("pong"), nil
}

// Terminate invoked on a termination supervisor process
func (sup *NexSupervisor) Terminate(reason error) {
	sup.Log().Info("supervisor terminated with reason: %s", reason)
}

// HandleInspect invoked on the request made with gen.Process.Inspect(...)
func (sup *NexSupervisor) HandleInspect(from gen.PID, item ...string) map[string]string {
	sup.Log().Info("supervisor got inspect request from %s", from)
	return nil
}