package lib

import (
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

// ELF execution provider implementation
type ELF struct {
	environment map[string]string
	name        string
	tmpFilename string
	totalBytes  int32
	vmID        string

	// started chan int

	stderr io.Writer
	stdout io.Writer
}

// Execute the ELF binary
func (e *ELF) Execute() error {
	// This has to be backgrounded because the workload could be a long-running process/service
	go func() {
		cmd := exec.Command(e.tmpFilename)
		cmd.Stdout = e.stdout
		cmd.Stderr = e.stderr

		envVars := make([]string, len(e.environment))
		for k, v := range e.environment {
			item := fmt.Sprintf("%s=%s", strings.ToUpper(k), v)
			envVars = append(envVars, item)
		}
		cmd.Env = envVars

		err := cmd.Start()

		if err != nil {
			// FIXME
			// msg := fmt.Sprintf("Failed to start workload: %s", err)
			// //e.agent.LogError(msg)
			// //e.agent.PublishWorkloadStopped(e.vmID, e.name, true, msg)
			// return
		}

		//e.agent.PublishWorkloadStarted(e.vmID, e.name, e.totalBytes)

		// if cmd.Wait has unblocked, it means the workload has stopped
		err = cmd.Wait()

		// FIXME
		// var msg string
		// if err != nil {
		// 	msg = fmt.Sprintf("Workload stopped unexpectedly: %s", err)
		// } else {
		// 	msg = "OK"
		// }

		//e.agent.PublishWorkloadStopped(e.vmID, e.name, err != nil, msg)

	}()

	return nil
}

// Validate the underlying artifact to be a 64-bit linux native ELF
// binary that is statically-linked
func (e *ELF) Validate() error {
	err := validateNativeBinary(e.tmpFilename)
	if err != nil {
		return err
	}

	return nil
}

// InitNexExecutionProviderELF convenience method to initialize an ELF execution provider
func InitNexExecutionProviderELF(params *agentapi.ExecutionProviderParams) *ELF {
	return &ELF{
		environment: params.Environment,
		name:        params.WorkloadName,
		tmpFilename: params.TmpFilename,
		totalBytes:  int32(params.TotalBytes),
		vmID:        params.VmID,

		stderr: params.Stderr,
		stdout: params.Stdout,
	}
}

// Validates that the indicated file is a 64-bit linux native elf binary that is statically linked.
// All native binaries must pass this validation before they are executed.
func validateNativeBinary(path string) error {
	elfFile, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open source binary: %s", err)
	}
	defer elfFile.Close()

	err = verifyStatic(elfFile)
	if err != nil {
		return fmt.Errorf("file failed static link check: %s", err)
	}

	return nil
}

// verifyStatic returns an error if the provided elf binary contains any dynamically-linked dependencies
func verifyStatic(elf *elf.File) error {
	for _, prog := range elf.Progs {
		if prog.ProgHeader.Type == 3 { // PT_INTERP
			return errors.New("elf binary contains at least one dynamically linked dependency")
		}
	}
	return nil
}
