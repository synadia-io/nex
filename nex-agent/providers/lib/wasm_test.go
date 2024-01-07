package lib

import (
	"testing"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

func TestWasmExecution(t *testing.T) {
	file := "../../../examples/wasm/echofunction/echofunction.wasm"
	typ := "wasm"
	params := &agentapi.ExecutionProviderParams{
		WorkRequest: agentapi.WorkRequest{
			Environment:  map[string]string{},
			Hash:         new(string),
			TotalBytes:   new(int32),
			WorkloadName: new(string),
			WorkloadType: new(string),
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
	}
	params.WorkRequest.WorkloadType = &typ
	wasm, err := InitNexExecutionProviderWasm(params)
	if err != nil {
		t.Fatalf("Failed to instantiate wasm provider: %s", err)
	}

	input := []byte("Hello world")
	subject := "test.trigger"

	output, err := wasm.RunTrigger(subject, input)
	if err != nil {
		t.Fatalf("Failed to run trigger: %s", err)
	}

	if string(output[:]) != "Hello worldtest.trigger" {
		t.Fatalf("wasm module did not return the right data")
	}
}
