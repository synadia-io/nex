package test

import (
	"context"
	"testing"

	"github.com/synadia-io/nex/agent/providers/lib"
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

func TestWasmExecution(t *testing.T) {
	file := "../examples/wasm/echofunction/echofunction.wasm"
	typ := controlapi.NexWorkloadWasm
	params := &agentapi.ExecutionProviderParams{
		AgentWorkloadInfo: agentapi.AgentWorkloadInfo{
			Environment:  map[string]string{},
			Hash:         "",
			TotalBytes:   0,
			WorkloadName: new(string),
			WorkloadType: typ,
			Stderr:       nil,
			Stdout:       nil,
			TmpFilename:  &file,
			Errors:       []error{},
		},
		Fail:        make(chan bool),
		Run:         make(chan bool),
		Exit:        make(chan int),
		Stderr:      nil,
		Stdout:      nil,
		TmpFilename: &file,
		VmID:        "bob",

		NATSConn: nil, // FIXME
	}
	params.AgentWorkloadInfo.WorkloadType = typ
	wasm, err := lib.InitNexExecutionProviderWasm(params)
	if err != nil {
		t.Fatalf("Failed to instantiate wasm provider: %s", err)
	}

	_ = wasm.Validate()
	_ = wasm.Deploy()

	input := []byte("Hello world")
	subject := "test.trigger"

	ctx := context.WithValue(context.Background(), agentapi.NexTriggerSubject, subject) //nolint:all
	output, err := wasm.Execute(ctx, input)
	if err != nil {
		t.Fatalf("Failed to run trigger: %s", err)
	}

	if string(output[:]) != "Hello worldtest.trigger" {
		t.Fatalf("wasm module did not return the right data")
	}
}
