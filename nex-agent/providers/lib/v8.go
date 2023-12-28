package lib

import (
	"fmt"
	"os"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	v8 "rogchap.com/v8go"
)

const v8MaxFileSizeBytes = int64(12288) // arbitrarily ~12K, for now

// V8 execution provider implementation
type V8 struct {
	environment map[string]string
	name        string
	tmpFilename string
	totalBytes  int32
	vmID        string

	fail chan bool
	run  chan bool
	exit chan int

	// stderr io.Writer
	// stdout io.Writer

	ctx *v8.Context
	ubs *v8.UnboundScript
}

// Execute expects a `Validate` to have succeeded and `ubs` to be non-nil
func (v *V8) Execute() error {
	if v.ubs == nil {
		return fmt.Errorf("invalid state for execution; no compiled code available for vm: %s", v.name)
	}

	// TODO-- implement the following
	// cmd.Env = make([]string, len(e.environment))
	// for k, v := range e.environment {
	// 	item := fmt.Sprintf("%s=%s", strings.ToUpper(k), v)
	// 	cmd.Env = append(cmd.Env, item)
	// }

	var err error

	// vals := make(chan *v8.Value, 1)
	errs := make(chan error, 1)

	go func() {
		_, err = v.ubs.Run(v.ctx)

		if err != nil {
			errs <- err
		}
	}()

	select {
	// case <-vals:
	// we don't care about any returned values... executed scripts should return nothing...
	case <-errs:
		// javascript error
	case <-time.After(time.Millisecond * agentapi.DefaultRunloopSleepTimeoutMillis):
		if err != nil {
			// TODO-- check for v8.JSError as this type has Message, Location and StackTrace we can log...
			return fmt.Errorf("failed to invoke default export: %s", err)
		}

		v.run <- true
	}

	return nil
}

// Validate has the side effect of compiling the executable javascript source
// code and setting `ubs` on the underlying V8 execution provider instance.
func (v *V8) Validate() error {
	if v.ctx == nil {
		return fmt.Errorf("invalid state for validation; no v8 context available for vm: %s", v.name)
	}

	f, err := os.Open(v.tmpFilename)
	if err != nil {
		return fmt.Errorf("failed to open source: %s", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat open source file: %s", err)
	}

	if fi.Size() > v8MaxFileSizeBytes {
		return fmt.Errorf("source file (%d bytes) exceeds maximum of %d bytes", fi.Size(), v8MaxFileSizeBytes)
	}

	src, err := os.ReadFile(v.tmpFilename)
	if err != nil {
		return fmt.Errorf("failed to open source for execution: %s", err)
	}

	v.ubs, err = v.ctx.Isolate().CompileUnboundScript(string(src), v.tmpFilename, v8.CompileOptions{})
	if err != nil {
		return fmt.Errorf("failed to compile source for execution: %s", err)
	}

	return nil
}

// convenience method to initialize a V8 execution provider
func InitNexExecutionProviderV8(params *agentapi.ExecutionProviderParams) *V8 {
	return &V8{
		environment: params.Environment,
		name:        params.WorkloadName,
		tmpFilename: params.TmpFilename,
		totalBytes:  params.TotalBytes,
		vmID:        params.VmID,

		// stderr: params.Stderr,
		// stdout: params.Stdout,

		fail: params.Fail,
		run:  params.Run,
		exit: params.Exit,

		ctx: v8.NewContext(),
	}
}
