package lib

import (
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

// NativeExecutable execution provider implementation
type NativeExecutable struct {
	argv        []string
	environment map[string]string
	name        string
	tmpFilename string
	totalBytes  int64
	vmID        string

	fail     chan bool
	run      chan bool
	exit     chan int
	undeploy sync.Once

	cmd *exec.Cmd

	stderr io.Writer
	stdout io.Writer
}

// Deploy the ELF binary
func (e *NativeExecutable) Deploy() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("deploy recovered from panic")
		}
	}()

	cmd := exec.Command(e.tmpFilename, e.argv...)
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr
	cmd.SysProcAttr = e.sysProcAttr()

	cmd.Env = make([]string, len(e.environment))
	for k, v := range e.environment {
		item := fmt.Sprintf("%s=%s", strings.ToUpper(k), v)
		cmd.Env = append(cmd.Env, item)
	}

	err = cmd.Start()
	if err != nil {
		e.fail <- true
		return
	}

	e.cmd = cmd

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
			if cmd.ProcessState != nil {
				e.exit <- cmd.ProcessState.ExitCode()
			} else {
				e.exit <- 1
			}
		}
	}()

	return
}

func (e *NativeExecutable) removeWorkload() {
	_ = os.Remove(e.tmpFilename)
}

func (e *NativeExecutable) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	return nil, errors.New("Native execution provider does not support execution via trigger subjects")
}

// Validate the underlying artifact to be a 64-bit linux native ELF
// binary that is statically-linked
func (e *NativeExecutable) Validate() error {
	return validateNativeBinary(e.tmpFilename)
}

// convenience method to initialize an ELF execution provider
func InitNexExecutionProviderNative(params *agentapi.ExecutionProviderParams) (*NativeExecutable, error) {
	if params.WorkloadName == nil {
		return nil, errors.New("Native execution provider requires a workload name parameter")
	}

	if params.TmpFilename == nil {
		return nil, errors.New("Native execution provider requires a temporary filename parameter")
	}

	return &NativeExecutable{
		argv:        params.Argv,
		environment: params.Environment,
		name:        *params.WorkloadName,
		tmpFilename: *params.TmpFilename,
		totalBytes:  params.TotalBytes,
		vmID:        params.VmID,

		stderr: params.Stderr,
		stdout: params.Stdout,

		fail: params.Fail,
		run:  params.Run,
		exit: params.Exit,
	}, nil
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
