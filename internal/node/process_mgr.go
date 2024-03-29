package nexnode

import (
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

// A process manager is responsible for stopping and starting a Nex Agent. It is entirely up to
// the implementation of the process manager as to whether or to what degree any kind of sandboxing
// (e.g. firecracker) is employed. Note that agent processes are created asynchronously -before- any
// workloads are deployed to them, so a workload manager can never explicitly tell a process manager
// to create an individual process
type ProcessManager interface {
	// Starts the process manager. This allows the process manager to create a pool of agents however it sees fit
	Start(ProcessSubscriber) error
	// Stops the process manager and all running processes within
	Stop() error
	// Tells the process manager to associate the deployment request with the given workload ID, and perform any
	// just in time initialization of resources if necessary
	PrepareWorkload(string, *agentapi.DeployRequest) error
	// Requests that the process manager terminate the process with the given ID
	StopProcess(string) error
	// Performs a lookup on a process to return its associated deployment request. This will return nil if done
	// on an "unprepared" workload
	Lookup(string) (*agentapi.DeployRequest, error)
	// Returns a list of processes in an implementation-agnostic format
	ListProcesses() ([]ProcessInfo, error)
}

// A process subscriber is any struct that wishes too be notified when a process manager has
// finished starting a process
type ProcessSubscriber interface {
	// Indicates that the process is now ready, and can be "prepared" for deployment
	OnProcessReady(workloadId string)
}

// Information on a process without regard to the implementation of the process manager
type ProcessInfo struct {
	ID              string
	Name            string
	Namespace       string
	OriginalRequest *agentapi.DeployRequest
}
