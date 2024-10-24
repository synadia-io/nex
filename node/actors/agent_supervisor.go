package actors

import (
	"errors"
	"fmt"
	"log/slog"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/synadia-io/nex/models"
)

func createAgentSupervisor() gen.ProcessBehavior {
	return &agentSupervisor{}
}

// Agent manager is responsible for starting one agent per workload type, supplying it with
// the right parameters, and keeping track of the information received from its initial
// registration
type agentSupervisor struct {
	act.Supervisor

	nodeOptions models.NodeOptions
}

type agentSupervisorParams struct {
	options models.NodeOptions
}

func (p *agentSupervisorParams) Validate() error {
	var err error

	// insert validations
	// validate options much?

	return err
}

// Init invoked on a spawn Supervisor process. This is a mandatory callback for the implementation
func (mgr *agentSupervisor) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec
	if len(args) != 1 {
		err := errors.New("agent manager params are required")
		mgr.Log().Error("Failed to start agent manager", slog.String("error", err.Error()))
		fmt.Println(err)
		return spec, gen.TerminateReasonPanic
	}

	params, ok := args[0].(agentSupervisorParams)
	if !ok {
		err := errors.New("args[0] must be valid agent manager params")
		mgr.Log().Error("Failed to start agent manager", slog.String("error", err.Error()))
		fmt.Println(err)
		return spec, gen.TerminateReasonPanic
	}

	err := params.Validate()
	if err != nil {
		mgr.Log().Error("Failed to validate agent supervisor params", slog.String("error", err.Error()))
		fmt.Println(err)
		return spec, gen.TerminateReasonPanic
	}

	mgr.nodeOptions = params.options

	aTypes := []string{}
	if !mgr.nodeOptions.DisableDirectStart {
		spec.Children = []act.SupervisorChildSpec{
			{
				Name:    "directStartAgent",
				Factory: createDirectStartAgent,
				Args:    []any{},
			},
		}
		aTypes = append(aTypes, "direct_start")
	}

	for _, agent := range mgr.nodeOptions.AgentOptions {
		spec.Children = append(spec.Children, act.SupervisorChildSpec{
			Name:    gen.Atom(agent.Name),
			Factory: createExternalAgent,
			Args:    []any{externalAgentParams{agentOptions: agent}},
		})
		aTypes = append(aTypes, agent.Name)
	}

	mgr.Log().Info("started agent binaries", slog.Any("agents", aTypes))

	spec.Type = act.SupervisorTypeOneForOne
	spec.Restart.Strategy = act.SupervisorStrategyTransient
	spec.Restart.Intensity = 2 // How big bursts of restarts you want to tolerate.
	spec.Restart.Period = 5    // In seconds.

	return spec, nil
}

// at some point there will be some HandleCall handlers in here where other processes
// do process.Call("agent_manager", ...) in order to query information about running
// agents, etc.

// HandleInspect invoked on the request made with gen.Process.Inspect(...)
func (mgr *agentSupervisor) HandleInspect(from gen.PID, item ...string) map[string]string {
	mgr.Log().Info("agent manager got inspect request from %s", from)
	return nil
}
