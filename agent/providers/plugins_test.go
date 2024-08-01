//go:build linux && !race

package providers

import (
	"fmt"
	"testing"

	"github.com/nats-io/nkeys"
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

func TestNoopPluginLoad(t *testing.T) {
	plugPath := "../../test/fixtures"
	wName := "echofunctionjs"

	recipientKey, _ := nkeys.CreateCurveKeys()

	params := &agentapi.ExecutionProviderParams{
		DeployRequest: controlapi.DeployRequest{
			WorkloadName: &wName,
			WorkloadType: "noop",
		},
		Fail:        make(chan bool),
		Run:         make(chan bool),
		Exit:        make(chan int),
		Stderr:      nil,
		Stdout:      nil,
		TmpFilename: new(string),
		VmID:        "bob",
		PluginPath:  &plugPath,
	}

	prov, err := MaybeLoadPluginProvider(params, recipientKey)
	if err != nil {
		t.Fatalf("Failed to create plugin provider: %s", err.Error())
	}
	fmt.Printf("%s\n", prov.Name())
	fmt.Printf("%+v\n", prov)
}
