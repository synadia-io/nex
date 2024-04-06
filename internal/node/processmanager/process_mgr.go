package processmanager

import (
	"context"
	"log/slog"

	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"github.com/synadia-io/nex/internal/models"
	"github.com/synadia-io/nex/internal/node/observability"
)

// Information about an agent process without regard to the implementation of the agent process manager
type ProcessInfo struct {
	DeployRequest *agentapi.DeployRequest
	ID            string
	Name          string
	Namespace     string
}

// A process delegate is any struct that wishes to be notified when the configured agent process
// manager has successfully started an agent
type ProcessDelegate interface {
	// Indicates that an agent process with the given id has been started and is ready to be "prepared" for workload deployment
	OnProcessStarted(id string)
}

// A process manager is responsible for stopping and starting a Nex Agent. It is entirely up to
// the implementation of the process manager as to whether or to what degree any kind of sandboxing
// (e.g. firecracker) is employed. Note that agent processes are created asynchronously -before- any
// workloads are deployed to them, so a workload manager can never explicitly tell a process manager
// to create an individual process
type ProcessManager interface {
	// Returns a list of agent processes in an implementation-agnostic format
	ListProcesses() ([]ProcessInfo, error)

	// Lookup a deploy request by id. Returns nil when attempting to lookup an "unprepared" workload
	Lookup(id string) (*agentapi.DeployRequest, error)

	// Associate a deploy request with the given workload id, and perform any
	// just in time initialization of resources if necessary
	PrepareWorkload(id string, request *agentapi.DeployRequest) error

	// Start the process manager and allocate a pool of agents based on an implementation-specific
	// strategy, delegating callbacks to the given delegate
	Start(delegate ProcessDelegate) error

	// Stop the process manager and gracefully shutdown all agents in the pool
	Stop() error

	// Terminate a running agent process with the given ID
	StopProcess(id string) error
}

// Initialie an appropriate agent process manager instance based on the sandbox config value
func NewProcessManager(
	log *slog.Logger,
	config *models.NodeConfiguration,
	telemetry *observability.Telemetry,
	ctx context.Context,
) (ProcessManager, error) {
	if config.NoSandbox {
		log.Warn("⚠️  Sandboxing has been disabled! Workloads are spawned directly by agents")
		log.Warn("⚠️  Do not run untrusted workloads in this mode!")
		return NewSpawningProcessManager(log, config, telemetry, ctx)
	}

	return NewFirecrackerProcessManager(log, config, telemetry, ctx)
}
