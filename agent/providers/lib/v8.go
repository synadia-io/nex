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
	hostServicesObjectName = "hostServices"

	hostServicesKVObjectName         = "kv"
	hostServicesKVGetFunctionName    = "get"
	hostServicesKVSetFunctionName    = "set"
	hostServicesKVDeleteFunctionName = "delete"
	hostServicesKVKeysFunctionName   = "keys"

	hostServicesKVGetTimeout    = time.Millisecond * 250
	hostServicesKVSetTimeout    = time.Millisecond * 250
	hostServicesKVDeleteTimeout = time.Millisecond * 250
	hostServicesKVKeysTimeout   = time.Millisecond * 250

	hostServicesMessagingObjectName              = "messaging"
	hostServicesMessagingPublishFunctionName     = "publish"
	hostServicesMessagingRequestFunctionName     = "request"
	hostServicesMessagingRequestManyFunctionName = "requestMany"

	hostServicesMessagingPublishTimeout     = time.Millisecond * 500
	hostServicesMessagingRequestTimeout     = time.Millisecond * 500
	hostServicesMessagingRequestManyTimeout = time.Millisecond * 3000

	nexTriggerSubject = "x-nex-trigger-subject"
	nexRuntimeNs      = "x-nex-runtime-ns"

	messageSubject = "x-subject"

	v8FunctionArrayAppend = "array-append"
	v8FunctionArrayInit   = "array-init"

	v8ExecutionTimeoutMillis = 5000
	v8MaxFileSizeBytes       = int64(12288) // arbitrarily ~12K, for now
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

	ctx   *v8.Context // default context for internal use only
	iso   *v8.Isolate
	ubs   *v8.UnboundScript
	utils map[string]*v8.Function //v8.UnboundScript
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

	ctx, err := v.newV8Context()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize context in vm: %s", err.Error())
	}

	vals := make(chan *v8.Value, 1)
	errs := make(chan error, 1)

	go func() {
		val, err := v.ubs.Run(ctx)
		if err != nil {
			errs <- err
			return
		}

		fn, err := val.AsFunction()
		if err != nil {
			errs <- err
			return
		}

		argv1, err := v8.NewValue(ctx.Isolate(), subject)
		if err != nil {
			errs <- err
			return
		}

		argv2, err := v8.NewValue(ctx.Isolate(), string(payload))
		if err != nil {
			errs <- err
			return
		}

		val, err = fn.Call(ctx.Global(), argv1, argv2)
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
		_, _ = v.stderr.Write([]byte(fmt.Sprintf("v8 execution failed with error: %s", err.Error())))
		return nil, err
	case <-time.After(time.Millisecond * v8ExecutionTimeoutMillis):
		// if err != nil {
		// }

		return nil, fmt.Errorf("v8 execution timed out after %dms", v8ExecutionTimeoutMillis)
	}
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
		return fmt.Errorf("invalid state for validation; default v8 context already initialized for vm: %s", v.name)
	}

	v.ctx = v8.NewContext(v.iso) // default context for internal use only

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

	v.ubs, err = v.iso.CompileUnboundScript(string(src), v.tmpFilename, v8.CompileOptions{})
	if err != nil {
		return fmt.Errorf("failed to compile source for execution: %s", err)
	}

	// FIXME-- move this somewhere cleaner
	append, _ := v.iso.CompileUnboundScript("(arr, value) => { arr.push(value); return arr; };", "array-append.js", v8.CompileOptions{})
	appendval, _ := append.Run(v.ctx)
	appendfn, _ := appendval.AsFunction()
	v.utils[v8FunctionArrayAppend] = appendfn

	// FIXME-- move this somewhere cleaner
	init, _ := v.iso.CompileUnboundScript("() => { let arr = []; return arr; };", "array-init.js", v8.CompileOptions{})
	initval, _ := init.Run(v.ctx)
	initfn, _ := initval.AsFunction()
	v.utils[v8FunctionArrayInit] = initfn

	return nil
}

func (v *V8) newV8Context() (*v8.Context, error) {
	global := v8.NewObjectTemplate(v.iso)

	hostServices, err := v.newHostServicesTemplate()
	if err != nil {
		return nil, err
	}

	err = global.Set(hostServicesObjectName, hostServices)
	if err != nil {
		return nil, err
	}

	return v8.NewContext(v.iso, global), nil
}

// agentint.{vmID}.rpc.{namespace}.{workload}.kv.{method}
func (v *V8) keyValueServiceSubject(method string) string {
	return fmt.Sprintf("agentint.%s.rpc.%s.%s.kv.%s", v.vmID, v.namespace, v.name, method)
}

// agentint.{vmID}.rpc.{namespace}.{workload}.messaging.{method}
func (v *V8) messagingServiceSubject(method string) string {
	return fmt.Sprintf("agentint.%s.rpc.%s.%s.messaging.%s", v.vmID, v.namespace, v.name, method)
}

func (v *V8) newHostServicesTemplate() (*v8.ObjectTemplate, error) {
	hostServices := v8.NewObjectTemplate(v.iso)

	err := hostServices.Set(hostServicesKVObjectName, v.newKeyValueObjectTemplate())
	if err != nil {
		return nil, err
	}

	err = hostServices.Set(hostServicesMessagingObjectName, v.newMessagingObjectTemplate())
	if err != nil {
		return nil, err
	}

	return hostServices, nil
}

func (v *V8) newKeyValueObjectTemplate() *v8.ObjectTemplate {
	kv := v8.NewObjectTemplate(v.iso)

	_ = kv.Set(hostServicesKVGetFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) != 1 {
			val, _ := v8.NewValue(v.iso, "key is required")
			return v.iso.ThrowException(val)
		}

		key := args[0].String()

		req, _ := json.Marshal(&agentapi.HostServicesKeyValueRequest{
			Key: &key,
		})

		resp, err := v.nc.Request(v.keyValueServiceSubject(hostServicesKVGetFunctionName), req, hostServicesKVGetTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var kvresp *agentapi.HostServicesKeyValueRequest
		err = json.Unmarshal(resp.Data, &kvresp)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		val, err := v8.JSONParse(v.ctx, string(*kvresp.Value))
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		return val
	}))

	_ = kv.Set(hostServicesKVSetFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) != 2 {
			val, _ := v8.NewValue(v.iso, "key and value are required")
			return v.iso.ThrowException(val)
		}

		key := args[0].String()
		value := args[1]

		raw, err := value.MarshalJSON()
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		val := json.RawMessage(raw)
		req, _ := json.Marshal(&agentapi.HostServicesKeyValueRequest{
			Key:   &key,
			Value: &val,
		})

		resp, err := v.nc.Request(v.keyValueServiceSubject(hostServicesKVSetFunctionName), req, hostServicesKVSetTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var kvresp *agentapi.HostServicesKeyValueRequest
		err = json.Unmarshal(resp.Data, &kvresp)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		if !*kvresp.Success {
			val, _ := v8.NewValue(v.iso, fmt.Sprintf("failed to set %d-byte value for key: %s", len(val), key))
			return v.iso.ThrowException(val)
		}

		return nil
	}))

	_ = kv.Set(hostServicesKVDeleteFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) != 1 {
			val, _ := v8.NewValue(v.iso, "key is required")
			return v.iso.ThrowException(val)
		}

		key := args[0].String()

		req, _ := json.Marshal(&agentapi.HostServicesKeyValueRequest{
			Key: &key,
		})

		resp, err := v.nc.Request(v.keyValueServiceSubject(hostServicesKVDeleteFunctionName), req, hostServicesKVDeleteTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var kvresp *agentapi.HostServicesKeyValueRequest
		err = json.Unmarshal(resp.Data, &kvresp)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		if !*kvresp.Success {
			val, _ := v8.NewValue(v.iso, fmt.Sprintf("failed to delete key: %s", key))
			return v.iso.ThrowException(val)
		}

		return nil
	}))

	_ = kv.Set(hostServicesKVKeysFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		req, _ := json.Marshal(map[string]interface{}{})

		resp, err := v.nc.Request(v.keyValueServiceSubject(hostServicesKVKeysFunctionName), req, hostServicesKVKeysTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		val, err := v8.JSONParse(v.ctx, string(resp.Data))
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		return val
	}))

	return kv
}

func (v *V8) newMessagingObjectTemplate() *v8.ObjectTemplate {
	messaging := v8.NewObjectTemplate(v.iso)

	_ = messaging.Set(hostServicesMessagingPublishFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) != 2 {
			val, _ := v8.NewValue(v.iso, "subject and payload are required")
			return v.iso.ThrowException(val)
		}

		subject := args[0].String()
		payload := args[1].String()

		msg := nats.NewMsg(v.messagingServiceSubject(hostServicesMessagingPublishFunctionName))
		msg.Header.Add(messageSubject, subject)
		msg.Data = []byte(payload)

		resp, err := v.nc.RequestMsg(msg, hostServicesMessagingPublishTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var msgresp *agentapi.HostServicesMessagingResponse
		err = json.Unmarshal(resp.Data, &msgresp)
		if err == nil && len(msgresp.Errors) > 0 {
			val, _ := v8.NewValue(v.iso, msgresp.Errors[0])
			return v.iso.ThrowException(val)
		}

		return nil
	}))

	_ = messaging.Set(hostServicesMessagingRequestFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) != 2 {
			val, _ := v8.NewValue(v.iso, "subject and payload are required")
			return v.iso.ThrowException(val)
		}

		subject := args[0].String()
		payload := args[1].String()

		msg := nats.NewMsg(v.messagingServiceSubject(hostServicesMessagingRequestFunctionName))
		msg.Header.Add(messageSubject, subject)
		msg.Data = []byte(payload)

		resp, err := v.nc.RequestMsg(msg, hostServicesMessagingRequestTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var msgresp *agentapi.HostServicesMessagingResponse
		err = json.Unmarshal(resp.Data, &msgresp)
		if err == nil && len(msgresp.Errors) > 0 {
			val, _ := v8.NewValue(v.iso, msgresp.Errors[0])
			return v.iso.ThrowException(val)
		}

		val, err := v8.NewValue(v.iso, string(resp.Data)) // FIXME-- pass []byte natively into javascript using ArrayBuffer
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		return val
	}))

	_ = messaging.Set(hostServicesMessagingRequestManyFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) != 2 {
			val, _ := v8.NewValue(v.iso, "subject and payload are required")
			return v.iso.ThrowException(val)
		}

		subject := args[0].String()
		payload := args[1].String()

		// construct the requestMany request message
		msg := nats.NewMsg(v.messagingServiceSubject(hostServicesMessagingRequestManyFunctionName))
		msg.Header.Add(messageSubject, subject)
		msg.Reply = v.nc.NewRespInbox()
		msg.Data = []byte(payload)

		// create a synchronous subscription
		sub, err := v.nc.SubscribeSync(msg.Reply)
		if err != nil {
			_, _ = v.stderr.Write([]byte(fmt.Sprintf("failed to subscribe sync: %s", err.Error())))

			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		defer func() {
			_ = sub.Unsubscribe()
		}()

		_ = v.nc.Flush()

		// publish the requestMany request to the target subject
		err = v.nc.PublishMsg(msg)
		if err != nil {
			_, _ = v.stderr.Write([]byte(fmt.Sprintf("failed to publish message: %s", err.Error())))

			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		val, err := v.utils[v8FunctionArrayInit].Call(v.ctx.Global())
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		start := time.Now()
		for time.Since(start) < hostServicesMessagingRequestManyTimeout {
			resp, err := sub.NextMsg(hostServicesMessagingRequestTimeout)
			if err != nil && !errors.Is(err, nats.ErrTimeout) {
				val, _ := v8.NewValue(v.iso, err.Error())
				return v.iso.ThrowException(val)
			}

			if resp != nil {
				_, _ = v.stdout.Write([]byte(fmt.Sprintf("received %d-byte response", len(resp.Data))))

				respval, err := v8.NewValue(v.iso, string(resp.Data)) // FIXME-- pass []byte natively into javascript using ArrayBuffer
				if err != nil {
					val, _ := v8.NewValue(v.iso, err.Error())
					return v.iso.ThrowException(val)
				}

				val, err = v.utils[v8FunctionArrayAppend].Call(v.ctx.Global(), val, respval)
				if err != nil {
					val, _ := v8.NewValue(v.iso, err.Error())
					return v.iso.ThrowException(val)
				}
			}
		}

		return val
	}))

	return messaging
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

		utils: make(map[string]*v8.Function),
	}, nil
}
