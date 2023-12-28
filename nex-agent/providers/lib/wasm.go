package lib

import (
	"errors"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

// Wasm execution provider implementation
type Wasm struct {
}

func (e *Wasm) Execute() error {
	return errors.New("wasm execution provider not yet implemented")
}

func (e *Wasm) Validate() error {
	return errors.New("wasm execution provider not yet implemented")
}

// InitNexExecutionProviderWasm convenience method to initialize a Wasm execution provider
func InitNexExecutionProviderWasm(params *agentapi.ExecutionProviderParams) *Wasm {
	// TODO-- impl...
	return nil
}
