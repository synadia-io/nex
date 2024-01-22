package lib

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
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

	nc *nats.Conn // agent NATS connection

	ctx *v8.Context
	ubs *v8.UnboundScript
}

// Deploy expects a `Validate` to have succeeded and `ubs` to be non-nil
func (v *V8) Deploy() error {
	if v.ubs == nil {
		return fmt.Errorf("invalid state for execution; no compiled code available for vm: %s", v.name)
	}

	subject := fmt.Sprintf("agentint.%s.trigger", v.vmID)
	_, err := v.nc.Subscribe(subject, func(msg *nats.Msg) {
		val, err := v.Execute(msg.Header.Get("x-nex-trigger-subject"), msg.Data)
		if err != nil {
			// TODO-- propagate this error to agent logs
			return
		}

		if len(val) > 0 {
			_ = msg.Respond(val)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to trigger: %s", err)
	}

	v.run <- true
	return nil
}

// Trigger execution of the deployed function; expects a `Validate` to have succeeded and `ubs` to be non-nil.
// The executed function can optionally return a value, in which case it will be deemed a reply and returned
// to the caller. In the case of a nil or empty value returned by the function, no reply will be sent.
func (v *V8) Execute(subject string, payload []byte) ([]byte, error) {
	if v.ubs == nil {
		return nil, fmt.Errorf("invalid state for execution; no compiled code available for vm: %s", v.name)
	}

	// TODO-- implement the following
	// cmd.Env = make([]string, len(e.environment))
	// for k, v := range e.environment {
	// 	item := fmt.Sprintf("%s=%s", strings.ToUpper(k), v)
	// 	cmd.Env = append(cmd.Env, item)
	// }

	var err error

	vals := make(chan *v8.Value, 1)
	errs := make(chan error, 1)

	go func() {
		val, err := v.ubs.Run(v.ctx)
		if err != nil {
			errs <- err
			return
		}

		fn, err := val.AsFunction()
		if err != nil {
			errs <- err
			return
		}

		argv1, err := v8.NewValue(v.ctx.Isolate(), subject)
		if err != nil {
			errs <- err
			return
		}

		argv2, err := v8.NewValue(v.ctx.Isolate(), string(payload))
		if err != nil {
			errs <- err
			return
		}

		val, err = fn.Call(v.ctx.Global(), argv1, argv2)
		if err != nil {
			errs <- err
			return
		}

		vals <- val
	}()

	select {
	case val := <-vals:
		retval, err := val.MarshalJSON()
		if err != nil {
			return nil, err
		}
		return retval, nil
	case err := <-errs:
		return nil, err
	case <-time.After(time.Millisecond * agentapi.DefaultRunloopSleepTimeoutMillis):
		if err != nil {
			// TODO-- check for v8.JSError as this type has Message, Location and StackTrace we can log...
			return nil, fmt.Errorf("failed to invoke default export: %s", err)
		}
	}

	return nil, nil
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
func InitNexExecutionProviderV8(params *agentapi.ExecutionProviderParams) (*V8, error) {
	if params.WorkloadName == nil {
		return nil, errors.New("V8 execution provider requires a workload name parameter")
	}

	if params.TmpFilename == nil {
		return nil, errors.New("V8 execution provider requires a temporary filename parameter")
	}

	// if params.TotalBytes == nil {
	// 	return nil, errors.New("V8 execution provider requires a total bytes parameter")
	// }

	return &V8{
		environment: params.Environment,
		name:        *params.WorkloadName,
		tmpFilename: *params.TmpFilename,
		totalBytes:  0, // FIXME
		vmID:        params.VmID,

		// stderr: params.Stderr,
		// stdout: params.Stdout,

		fail: params.Fail,
		run:  params.Run,
		exit: params.Exit,

		nc:  params.NATSConn,
		ctx: v8.NewContext(),
	}, nil
}
