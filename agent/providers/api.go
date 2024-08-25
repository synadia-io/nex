package providers

import (
	"errors"

	"github.com/synadia-io/nex/agent/providers/lib"
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

// ExecutionProvider implementations provide support for a specific
// execution environment pattern -- e.g., statically-linked ELF
// binaries, serverless JavaScript functions, OCI images, Wasm, etc.
type ExecutionProvider interface {
	// Deploy a service (e.g., "elf" and "oci" types) or executable function (e.g., "v8" and "wasm" types)
	Deploy() error

	// Undeploy a workload, giving it a chance to gracefully clean up after itself (if applicable)
	Undeploy() error

	// Validate the executable artifact, e.g., specific characteristics of a
	// statically-linked binary or raw source code, depending on provider implementation
	Validate() error

	// Returns a human-readable name for this provider
	Name() string
}

// NewExecutionProvider initializes and returns an execution provider for a given work request
func NewExecutionProvider(params *agentapi.ExecutionProviderParams,
	md *agentapi.MachineMetadata) (ExecutionProvider, error) {

	if len(params.WorkloadType) == 0 {
		return nil, errors.New("execution provider factory requires a workload type parameter")
	}

	if params.WorkloadType == controlapi.NexWorkloadNative {
		return lib.InitNexExecutionProviderNative(params)
	} else {
		return InitNexExecutionProviderPlugin(params, md)
	}
}
