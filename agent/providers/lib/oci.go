package lib

import (
	"errors"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

// OCI execution provider implementation
type OCI struct {
}

func (o *OCI) Deploy() error {
	return errors.New("oci execution provider not yet implemented")
}

func (o *OCI) Execute(headers nats.Header, payload []byte) ([]byte, error) {
	return nil, errors.New("oci execution provider does not support execution via trigger subjects")
}

func (o *OCI) Undeploy() error {
	return errors.New("oci execution provider not yet implemented")
}

func (o *OCI) Validate() error {
	return errors.New("oci execution provider not yet implemented")
}

// InitNexExecutionProviderOCI convenience method to initialize an OCI execution provider
func InitNexExecutionProviderOCI(params *agentapi.ExecutionProviderParams) *OCI {
	// TODO-- impl...
	return nil
}
