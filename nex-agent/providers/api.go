package providers

import (
	"errors"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	"github.com/ConnectEverything/nex/nex-agent/providers/lib"
)

// NexExecutionProviderELF Executable Linkable Format execution provider
const NexExecutionProviderELF = "elf"

// NexExecutionProviderV8 V8 execution provider
const NexExecutionProviderV8 = "v8"

// NexExecutionProviderOCI OCI execution provider
const NexExecutionProviderOCI = "oci"

// NexExecutionProviderWasm Wasm execution provider
const NexExecutionProviderWasm = "wasm"

// ExecutionProvider implementations provide support for a specific
// execution environment "persona" -- e.g., statically-linked ELF
// binaries, serverless JavaScript functions, OCI images, Wasm, etc.
type ExecutionProvider interface {
	// Execute is the equivalent to a language-specific main() entrypoint
	Execute() error

	// TODO-- add a method to cleanup after execution terminates
	// Clean() error

	// Validate the executable artifact, e.g., specific characteristics of a
	// statically-linked binary or raw source code, depending on provider implementation
	Validate() error
}

// NewExecutionProvider initializes and returns an execution provider for a given work request
func NewExecutionProvider(params *agentapi.ExecutionProviderParams) (ExecutionProvider, error) {
	if params.WorkloadType == "" { // FIXME-- should req.WorkloadType be a *string for better readability? e.g., json.Unmarshal will set req.Type == "" even if it is not provided.
		return nil, errors.New("execution provider factory requires a workload type parameter")
	}

	switch params.WorkloadType {
	case NexExecutionProviderELF:
		return lib.InitNexExecutionProviderELF(params), nil
	case NexExecutionProviderV8:
		return lib.InitNexExecutionProviderV8(params), nil
	case NexExecutionProviderOCI:
		// TODO-- return lib.InitNexExecutionProviderOCI(params), nil
		return nil, errors.New("oci execution provider not yet implemented")
	case NexExecutionProviderWasm:
		// TODO-- return lib.InitNexExecutionProviderWasm(params), nil
		return nil, errors.New("wasm execution provider not yet implemented")
	default:
		break
	}

	return nil, errors.New("invalid execution provider specified")
}
