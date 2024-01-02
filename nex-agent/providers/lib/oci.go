package lib

/*
 * Docker execution
 * Assumptions:
 * - nex-node is not responsible for running an OCI registry
 * - it is the operator's responsibility to ensure that the firecracker VM has access to the registry
 * - the full path to the OCI image is contained in the request's Location property
 * - this executor just issues the docker run command, setting host networking to true (the host is the firecracker VM)
 */

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

// OCI execution provider implementation
type OCI struct {
	params *agentapi.ExecutionProviderParams
}

func (o *OCI) Execute() error {

	cmd := o.generateDockerCommand()

	err := cmd.Start()
	if err != nil {
		o.params.Fail <- true
		return err
	}

	go func() {
		go func() {
			for {
				if cmd.Process != nil {
					o.params.Run <- true
					return
				}

				// TODO-- implement a timeout after which we dispatch e.fail

				time.Sleep(time.Millisecond * agentapi.DefaultRunloopSleepTimeoutMillis)
			}
		}()

		// This has to be backgrounded because the workload could be a long-running process/service
		if err = cmd.Wait(); err != nil { // blocking until exit
			if exitError, ok := err.(*exec.ExitError); ok {
				o.params.Exit <- exitError.ExitCode() // this is here for now for review but can likely be simplified to one line: `e.exit <- cmd.ProcessState.ExitCode()``
			}
		} else {
			o.params.Exit <- cmd.ProcessState.ExitCode()
		}
	}()

	return nil
}

func (o *OCI) Validate() error {
	// currently no image validation
	return nil
}

func (o *OCI) generateDockerCommand() *exec.Cmd {
	env_params := make([]string, (2*len(o.params.Environment))+5)
	env_params = append(env_params, "run")
	env_params = append(env_params, "--rm")
	env_params = append(env_params, "--network")
	env_params = append(env_params, "host")

	for k, v := range o.params.Environment {
		env_params = append(env_params, "-e")
		env_params = append(env_params, fmt.Sprintf("%s=%s", strings.ToUpper(k), v))
	}

	env_params = append(env_params, o.params.Location.String())

	cmd := exec.Command("docker", env_params...)
	cmd.Stdout = o.params.Stdout
	cmd.Stderr = o.params.Stderr

	return cmd
}

// InitNexExecutionProviderOCI convenience method to initialize an OCI execution provider
func InitNexExecutionProviderOCI(params *agentapi.ExecutionProviderParams) *OCI {

	return &OCI{params: params}
}
