package lib

import (
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

// ELF execution provider implementation
type ELF struct {
	environment map[string]string
	name        string
	tmpFilename string
	totalBytes  int32
	vmID        string

	fail chan bool
	run  chan bool
	exit chan int

	stderr io.Writer
	stdout io.Writer
}

// Execute the ELF binary
func (e *ELF) Execute() error {
	cmd := exec.Command(e.tmpFilename)
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr

	cmd.Env = make([]string, len(e.environment))
	for k, v := range e.environment {
		item := fmt.Sprintf("%s=%s", strings.ToUpper(k), v)
		cmd.Env = append(cmd.Env, item)
	}

	err := cmd.Start()
	if err != nil {
		e.fail <- true
		return err
	}

	go func() {
		go func() {
			for {
				if cmd.Process != nil {
					e.run <- true
					return
				}

				// TODO-- implement a timeout after which we dispatch e.fail

				time.Sleep(time.Millisecond * agentapi.DefaultRunloopSleepTimeoutMillis)
			}
		}()

		// This has to be backgrounded because the workload could be a long-running process/service
		if err = cmd.Wait(); err != nil { // blocking until exit
			if exitError, ok := err.(*exec.ExitError); ok {
				e.exit <- exitError.ExitCode() // this is here for now for review but can likely be simplified to one line: `e.exit <- cmd.ProcessState.ExitCode()``
			}
		} else {
			e.exit <- cmd.ProcessState.ExitCode()
		}
	}()

	return nil
}

// Validate the underlying artifact to be a 64-bit linux native ELF
// binary that is statically-linked
func (e *ELF) Validate() error {
	return validateNativeBinary(e.tmpFilename)
}

// convenience method to initialize an ELF execution provider
func InitNexExecutionProviderELF(params *agentapi.ExecutionProviderParams) *ELF {
	return &ELF{
		environment: params.Environment,
		name:        params.WorkloadName,
		tmpFilename: params.TmpFilename,
		totalBytes:  params.TotalBytes,
		vmID:        params.MachineMetadata.VmId,

		stderr: params.Stderr,
		stdout: params.Stdout,

		fail: params.Fail,
		run:  params.Run,
		exit: params.Exit,
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

// Returns an error if the provided elf binary contains any dynamically-linked dependencies
func verifyStatic(elf *elf.File) error {
	for _, prog := range elf.Progs {
		if prog.ProgHeader.Type == 3 { // PT_INTERP
			return errors.New("elf binary contains at least one dynamically linked dependency")
		}
	}
	return nil
}
