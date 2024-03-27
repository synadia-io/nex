package nexnode

import agentapi "github.com/synadia-io/nex/internal/agent-api"

// TODO: this will be utilized in a forthcoming PR
type ProcessManager interface {
	StartProcess(agentapi.DeployRequest) (string, error)
	StopProcess(string) error
	Lookup(string) (*agentapi.DeployRequest, error)
	Exists(string) bool
	ListProcesses([]ProcessInfo, error)
}

type ProcessInfo struct {
	ID   string
	Name string
}
