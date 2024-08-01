package test

import (
	"context"
	"testing"

	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/agent/providers/lib"
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

func TestWasmExecution(t *testing.T) {
	file := "../examples/wasm/echofunction/echofunction.wasm"

	myKey, _ := nkeys.CreateCurveKeys()
	recipientKey, _ := nkeys.CreateCurveKeys()
	recipientPk, _ := recipientKey.PublicKey()
	issuerAccount, _ := nkeys.CreateAccount()

	depRequest, err := controlapi.NewDeployRequest(
		controlapi.SenderXKey(myKey),
		controlapi.Issuer(issuerAccount),
		controlapi.WorkloadName("testworkload"),
		controlapi.WorkloadDescription("testy mctesto"),
		controlapi.EnvironmentValue("NATS_URL", "nats://127.0.0.1:4222"),
		controlapi.EnvironmentValue("TOP_SECRET_LUGGAGE", "12345"),
		controlapi.SenderXKey(myKey),
		controlapi.Issuer(issuerAccount),
		controlapi.Location("nats://MUHBUCKET/muhfile"),
		controlapi.TargetPublicXKey(recipientPk),
		controlapi.Hash("browns"),
		controlapi.Namespace("default"),
		controlapi.WorkloadType(controlapi.NexWorkloadWasm),
	)
	if err != nil {
		t.Fatalf("Failed to create deploy request: %s", err)
	}

	params := &agentapi.ExecutionProviderParams{
		DeployRequest: *depRequest,
		Fail:          make(chan bool),
		Run:           make(chan bool),
		Exit:          make(chan int),
		Stderr:        nil,
		Stdout:        nil,
		TmpFilename:   &file,
		VmID:          "bob",

		NATSConn: nil, // FIXME
	}
	params.DeployRequest.WorkloadType = controlapi.NexWorkloadWasm
	wasm, err := lib.InitNexExecutionProviderWasm(params, recipientKey)
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
