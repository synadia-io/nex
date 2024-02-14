package lib

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	v8 "rogchap.com/v8go"
)

const (
	hostServicesObjectName           = "hostServices"
	hostServicesKVObjectName         = "kv"
	hostServicesKVGetFunctionName    = "get"
	hostServicesKVSetFunctionName    = "set"
	hostServicesKVDeleteFunctionName = "delete"
	hostServicesKVKeysFunctionName   = "keys"

	nexTriggerSubject = "x-nex-trigger-subject"
	nexRuntimeNs      = "x-nex-runtime-ns"

	v8MaxFileSizeBytes = int64(12288) // arbitrarily ~12K, for now
)

// V8 execution provider implementation
type V8 struct {
	environment map[string]string
	name        string
	namespace   string
	tmpFilename string
	totalBytes  int32
	vmID        string

	fail chan bool
	run  chan bool
	exit chan int

	stderr io.Writer
	stdout io.Writer

	nc *nats.Conn // agent NATS connection

	ctx *v8.Context
	iso *v8.Isolate
	ubs *v8.UnboundScript
}

// Deploy expects a `Validate` to have succeeded and `ubs` to be non-nil
func (v *V8) Deploy() error {
	if v.ubs == nil {
		return fmt.Errorf("invalid state for execution; no compiled code available for vm: %s", v.name)
	}

	subject := fmt.Sprintf("agentint.%s.trigger", v.vmID)
	_, err := v.nc.Subscribe(subject, func(msg *nats.Msg) {
		startTime := time.Now()
		val, err := v.Execute(msg.Header.Get(nexTriggerSubject), msg.Data)
		if err != nil {
			_, _ = v.stderr.Write([]byte(fmt.Sprintf("failed to execute function on trigger subject %s: %s", subject, err.Error())))
			return
		}

		runtimeNanos := time.Since(startTime).Nanoseconds()
		err = msg.RespondMsg(&nats.Msg{
			Data: val,
			Header: nats.Header{
				nexRuntimeNs: []string{strconv.FormatInt(runtimeNanos, 10)},
			},
		})
		if err != nil {
			_, _ = v.stderr.Write([]byte(fmt.Sprintf("failed to write %d-byte response: %s", len(val), err.Error())))
			return
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

func (v *V8) Undeploy() error {
	// We shouldn't have to do anything here since the script "owns" no resources
	return nil
}

// Validate has the side effect of compiling the executable javascript source
// code and setting `ubs` on the underlying V8 execution provider instance.
func (v *V8) Validate() error {
	if v.iso == nil {
		return fmt.Errorf("invalid state for validation; v8 isolate not initialized for vm: %s", v.name)
	}

	if v.ctx != nil {
		return fmt.Errorf("invalid state for validation; v8 context already initialized for vm: %s", v.name)
	}

	err := v.initV8Context()
	if err != nil {
		return fmt.Errorf("failed to initialize v8 context: %s", err)
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
		return fmt.Errorf("failed to open source for validation: %s", err)
	}

	v.ubs, err = v.ctx.Isolate().CompileUnboundScript(string(src), v.tmpFilename, v8.CompileOptions{})
	if err != nil {
		return fmt.Errorf("failed to compile source for execution: %s", err)
	}

	return nil
}

func (v *V8) initV8Context() error {
	global := v8.NewObjectTemplate(v.iso)

	hostServices, err := v.newHostServicesTemplate()
	if err != nil {
		return err
	}

	err = global.Set(hostServicesObjectName, hostServices)
	if err != nil {
		return err
	}

	v.ctx = v8.NewContext(v.iso, global)

	return nil
}

// agentint.{vmID}.rpc.{namespace}.{workload}.{service}.{method}
func (v *V8) keyValueServiceSubject(method string) string {
	return fmt.Sprintf("agentint.%s.rpc.%s.%s.kv.%s", v.vmID, v.namespace, v.name, method)
}

func (v *V8) newHostServicesTemplate() (*v8.ObjectTemplate, error) {
	hostServices := v8.NewObjectTemplate(v.iso)
	kv := v8.NewObjectTemplate(v.iso)

	_ = kv.Set(hostServicesKVGetFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) != 1 {
			val, _ := v8.NewValue(v.iso, false)
			return val
		}

		key := args[0].String()

		req, _ := json.Marshal(&agentapi.HostServicesKeyValueRequest{
			Key: &key,
		})

		resp, err := v.nc.Request(v.keyValueServiceSubject(hostServicesKVGetFunctionName), req, time.Millisecond*250)
		if err != nil {
			// TODO- log
			// TODO- iso.ThrowException(nil)
			return nil
		}

		var kvresp *agentapi.HostServicesKeyValueRequest
		err = json.Unmarshal(resp.Data, &kvresp)
		if err != nil {
			// TODO- log
			// TODO- iso.ThrowException(nil)
			return nil
		}

		val, err := v8.JSONParse(v.ctx, string(*kvresp.Value))
		if err != nil {
			// TODO- iso.ThrowException(nil)
			return nil
		}

		return val
	}))

	_ = kv.Set(hostServicesKVSetFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) != 2 {
			val, _ := v8.NewValue(v.iso, false)
			return val
		}

		key := args[0].String()
		value := args[1]

		raw, err := value.MarshalJSON()
		if err != nil {
			// TODO- iso.ThrowException(nil)
			return nil
		}

		val := json.RawMessage(raw)
		req, _ := json.Marshal(&agentapi.HostServicesKeyValueRequest{
			Key:   &key,
			Value: &val,
		})

		resp, err := v.nc.Request(v.keyValueServiceSubject(hostServicesKVSetFunctionName), req, time.Millisecond*250)
		if err != nil {
			// TODO- log
			// TODO- iso.ThrowException(nil)
			return nil
		}

		var kvresp *agentapi.HostServicesKeyValueRequest
		err = json.Unmarshal(resp.Data, &kvresp)
		if err != nil {
			// TODO- log
			// TODO- iso.ThrowException(nil)
			return nil
		}

		if !*kvresp.Success {
			// TODO- iso.ThrowException(nil)
			return nil
		}

		return nil
	}))

	_ = kv.Set(hostServicesKVDeleteFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) != 1 {
			val, _ := v8.NewValue(v.iso, false)
			return val
		}

		key := args[0].String()

		req, _ := json.Marshal(&agentapi.HostServicesKeyValueRequest{
			Key: &key,
		})

		resp, err := v.nc.Request(v.keyValueServiceSubject(hostServicesKVDeleteFunctionName), req, time.Millisecond*250)
		if err != nil {
			// TODO- log
			// TODO- iso.ThrowException(nil)
			return nil
		}

		var kvresp *agentapi.HostServicesKeyValueRequest
		err = json.Unmarshal(resp.Data, &kvresp)
		if err != nil {
			// TODO- log
			// TODO- iso.ThrowException(nil)
			return nil
		}

		if !*kvresp.Success {
			// TODO- iso.ThrowException(nil)
			return nil
		}

		return nil
	}))

	err := hostServices.Set(hostServicesKVObjectName, kv)
	if err != nil {
		return nil, err
	}

	return hostServices, nil
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
		namespace:   *params.Namespace,
		tmpFilename: *params.TmpFilename,
		totalBytes:  0, // FIXME
		vmID:        params.VmID,

		stderr: params.Stderr,
		stdout: params.Stdout,

		fail: params.Fail,
		run:  params.Run,
		exit: params.Exit,

		nc:  params.NATSConn,
		ctx: nil,
		iso: v8.NewIsolate(),
	}, nil
}
