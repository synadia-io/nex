package lib

import (
	"errors"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

// Wasm execution provider implementation
type Wasm struct {
}

func (e *Wasm) Deploy() error {
	return errors.New("wasm execution provider not yet implemented")
}

func (e *Wasm) Execute(subject string, payload []byte) ([]byte, error) {
	return nil, errors.New("wasm execution provider does not support trigger execution... yet ;)")
}

func (e *Wasm) Validate() error {
	return errors.New("wasm execution provider not yet implemented")
}

// InitNexExecutionProviderWasm convenience method to initialize a Wasm execution provider
func InitNexExecutionProviderWasm(params *agentapi.ExecutionProviderParams) *Wasm {
	// TODO-- impl...
	return nil
}
