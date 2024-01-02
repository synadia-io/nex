package lib

import (
	"testing"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

func TestCommandGeneration(t *testing.T) {
	o := &OCI{
		md: &agentapi.MachineMetadata{
			VmId:            "vm1234567",
			NodeNatsAddress: "192.168.127.1",
			NodePort:        0,
			Message:         "ahoy there",
		},
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
			},
			Fail:        make(chan bool),
			Run:         make(chan bool),
			Exit:        make(chan int),
			Stderr:      nil,
			Stdout:      nil,
			TmpFilename: "",
			VmID:        "vm1234567",
		},
	}
	cmd := o.generateDockerCommand()
	s := cmd.String()
	// cmd generator adds a bunch of whitespace
	if s != "/usr/bin/docker          run --rm --network host -e VAR1=val1 -e VAR2=val2 192.168.127.1:5000/vm1234567" {
		t.Fatalf("Expected a proper docker command, got '%s'", s)
	}
}
