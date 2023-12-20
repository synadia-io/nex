package lib

import agentapi "github.com/ConnectEverything/nex/agent-api"

// OCI execution provider implementation
type OCI struct {
}

func (o *OCI) Execute() error {
	return nil
}

func (o *OCI) Validate() error {
	return nil
}

// InitNexExecutionProviderOCI convenience method to initialize an OCI execution provider
func InitNexExecutionProviderOCI(req *agentapi.ExecutionProviderParams) *OCI {
	// TODO-- impl...
	return nil
}
