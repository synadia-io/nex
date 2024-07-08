//go:build linux

package providers

import (
	"fmt"
	"testing"

	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

func TestNoopPluginLoad(t *testing.T) {
	plugPath := "../../test/fixtures"
	wName := "echofunctionjs"
	params := &agentapi.ExecutionProviderParams{
		DeployRequest: agentapi.DeployRequest{
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

	prov, err := MaybeLoadPluginProvider(params)
	if err != nil {
		t.Fatalf("Failed to create plugin provider: %s", err.Error())
	}
	fmt.Printf("%s\n", prov.Name())
	fmt.Printf("%+v\n", prov)
}
