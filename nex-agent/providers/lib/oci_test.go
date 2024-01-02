package lib

import (
	"net/url"
	"strings"
	"testing"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

func TestCommandGeneration(t *testing.T) {

	url, _ := url.Parse("private.registry.io/test")
	o := &OCI{
		params: &agentapi.ExecutionProviderParams{
			WorkRequest: agentapi.WorkRequest{
				WorkloadName: "echoservice",
				Hash:         "",
				TotalBytes:   0,
				Environment: map[string]string{
					"var1": "val1",
					"var2": "val2",
				},
				WorkloadType: "oci",
				Stderr:       nil,
				Stdout:       nil,
				TmpFilename:  "",
				Location:     *url,
			},
			Fail:        make(chan bool),
			Run:         make(chan bool),
			Exit:        make(chan int),
			Stderr:      nil,
			Stdout:      nil,
			TmpFilename: "",
			VmID:        "vm1234567",
			MachineMetadata: &agentapi.MachineMetadata{
				VmId:            "vm1234567",
				NodeNatsAddress: "192.168.127.1",
				NodePort:        0,
				Message:         "ahoy there",
			},
		},
	}
	cmd := o.generateDockerCommand()
	s := cmd.String()
	s2 := strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	// cmd generator adds a bunch of whitespace
	if s2 != "/usr/bin/docker run --rm --network host -e VAR1=val1 -e VAR2=val2 private.registry.io/test" {
		t.Fatalf("Expected a proper docker command, got '%s'", s)
	}
}
