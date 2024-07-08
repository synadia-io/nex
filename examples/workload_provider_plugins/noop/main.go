package main

import "context"

type noopProvider struct {
}

func (p *noopProvider) Deploy() error {
	return nil
}

// Execute a deployed function, if supported by the execution provider implementation (e.g., "v8" and "wasm" types)
func (p *noopProvider) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	return []byte{1, 2, 3}, nil
}

// Undeploy a workload, giving it a chance to gracefully clean up after itself (if applicable)
func (p *noopProvider) Undeploy() error {
	return nil
}

// Validate the executable artifact, e.g., specific characteristics of a
// statically-linked binary or raw source code, depending on provider implementation
func (p *noopProvider) Validate() error {
	return nil
}

func (p *noopProvider) Name() string {
	return "No-Op (Example)"
}

// exported
var ExecutionProvider noopProvider
