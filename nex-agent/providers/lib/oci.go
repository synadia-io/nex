package lib

import (
	"errors"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

// OCI execution provider implementation
type OCI struct {
}

func (o *OCI) Deploy() error {
	return errors.New("oci execution provider not yet implemented")
}

func (o *OCI) Execute(subject string, payload []byte) ([]byte, error) {
	return nil, errors.New("oci execution provider does not support execution via trigger subjects")
}

func (o *OCI) Validate() error {
	return errors.New("oci execution provider not yet implemented")
}

// InitNexExecutionProviderOCI convenience method to initialize an OCI execution provider
func InitNexExecutionProviderOCI(params *agentapi.ExecutionProviderParams) *OCI {
	// TODO-- impl...
	return nil
}
