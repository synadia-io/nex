package lib

import agentapi "github.com/ConnectEverything/nex/agent-api"

// V8 execution provider implementation
type V8 struct {
}

func (v *V8) Execute() error {
	return nil
}

func (v *V8) Validate() error {
	return nil
}

// InitNexExecutionProviderV8 convenience method to initialize a V8 execution provider
func InitNexExecutionProviderV8(req *agentapi.ExecutionProviderParams) *V8 {
	// TODO-- impl...
	return nil
}
