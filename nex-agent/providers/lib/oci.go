package lib

import (
	"errors"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

// OCI execution provider implementation
type OCI struct {
}

func (o *OCI) Execute() error {
	return errors.New("oci execution provider not yet implemented")
}

func (o *OCI) Validate() error {
	return errors.New("oci execution provider not yet implemented")
}

// InitNexExecutionProviderOCI convenience method to initialize an OCI execution provider
func InitNexExecutionProviderOCI(req *agentapi.ExecutionProviderParams) *OCI {
	// TODO-- impl...
	return nil
}
