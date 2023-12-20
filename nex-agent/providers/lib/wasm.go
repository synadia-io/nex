package lib

import agentapi "github.com/ConnectEverything/nex/agent-api"

// Wasm execution provider implementation
type Wasm struct {
}

func (e *Wasm) Execute() error {
	return nil
}

func (e *Wasm) Validate() error {
	return nil
}

// InitNexExecutionProviderWasm convenience method to initialize a Wasm execution provider
func InitNexExecutionProviderWasm(req *agentapi.ExecutionProviderParams) *Wasm {
	// TODO-- impl...
	return nil
}
