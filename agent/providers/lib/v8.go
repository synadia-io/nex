//go:build linux && amd64

package lib

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	v8 "rogchap.com/v8go"
)

const (
	hostServicesObjectName = "hostServices"

	hostServicesHTTPObjectName         = "http"
	hostServicesHTTPGetFunctionName    = "get"
	hostServicesHTTPPostFunctionName   = "post"
	hostServicesHTTPPutFunctionName    = "put"
	hostServicesHTTPPatchFunctionName  = "patch"
	hostServicesHTTPDeleteFunctionName = "delete"
	hostServicesHTTPHeadFunctionName   = "head"

	hostServicesHTTPRequestTimeout = time.Millisecond * 3000

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

	hostServicesObjectStoreObjectName         = "objectStore"
	hostServicesObjectStoreGetFunctionName    = "get"
	hostServicesObjectStorePutFunctionName    = "put"
	hostServicesObjectStoreDeleteFunctionName = "delete"
	hostServicesObjectStoreListFunctionName   = "list"

	hostServicesObjectStoreGetTimeout    = time.Millisecond * 3000
	hostServicesObjectStorePutTimeout    = time.Millisecond * 3000
	hostServicesObjectStoreDeleteTimeout = time.Millisecond * 3000
	hostServicesObjectStoreListTimeout   = time.Millisecond * 3000

	v8FunctionArrayAppend        = "array-append"
	v8FunctionArrayInit          = "array-init"
	v8FunctionUInt8ArrayInit     = "uint8-array-init"
	v8FunctionUInt8ArraySetIdx   = "uint8-array-set-idx"
	v8FunctionUInt8ArrayToArray  = "uint8-array-to-array"
	v8FunctionUInt8ArrayToString = "uint8-array-to-string"

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
		val, err := v.Execute(msg.Header.Get(agentapi.NexTriggerSubject), msg.Data)
		if err != nil {
			_, _ = v.stderr.Write([]byte(fmt.Sprintf("failed to execute function on trigger subject %s: %s", subject, err.Error())))
			return
		}

		runtimeNanos := time.Since(startTime).Nanoseconds()
		err = msg.RespondMsg(&nats.Msg{
			Data: val,
			Header: nats.Header{
				agentapi.NexRuntimeNs: []string{strconv.FormatInt(runtimeNanos, 10)},
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

		argv2, err := v.toUInt8ArrayValue(payload)
		if err != nil {
			_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to convert raw %d-length []byte to Uint8[]: %s", len(payload), err.Error())))
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
		// FIXME-- switch on val type or are we ok with forcing a JSON response?

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

	v.initUtils()

	return nil
}

func (v *V8) initUtils() {
	append, _ := v.iso.CompileUnboundScript("(arr, value) => { arr.push(value); return arr; };", "array-append.js", v8.CompileOptions{})
	appendval, _ := append.Run(v.ctx)
	appendfn, _ := appendval.AsFunction()
	v.utils[v8FunctionArrayAppend] = appendfn

	init, _ := v.iso.CompileUnboundScript("() => { let arr = []; return arr; };", "array-init.js", v8.CompileOptions{})
	initval, _ := init.Run(v.ctx)
	initfn, _ := initval.AsFunction()
	v.utils[v8FunctionArrayInit] = initfn

	inituint8, _ := v.iso.CompileUnboundScript("(len) => { let arr = new Uint8Array(Number(len)); return arr; };", "uint8-array-init.js", v8.CompileOptions{})
	inituint8val, _ := inituint8.Run(v.ctx)
	inituint8fn, _ := inituint8val.AsFunction()
	v.utils[v8FunctionUInt8ArrayInit] = inituint8fn

	uint8arrsetidx, _ := v.iso.CompileUnboundScript("(arr, i, value) => { arr[Number(i)] = value; return arr; };", "uint8-array-set-idx.js", v8.CompileOptions{})
	uint8arrsetidxval, _ := uint8arrsetidx.Run(v.ctx)
	uint8arrsetidxfn, _ := uint8arrsetidxval.AsFunction()
	v.utils[v8FunctionUInt8ArraySetIdx] = uint8arrsetidxfn

	uint8arrtoarr, _ := v.iso.CompileUnboundScript("(arr) => { return Array.prototype.slice.call(arr); };", "uint8-array-to-array.js", v8.CompileOptions{})
	uint8arrtoarrval, _ := uint8arrtoarr.Run(v.ctx)
	uint8arrtoarrfn, _ := uint8arrtoarrval.AsFunction()
	v.utils[v8FunctionUInt8ArrayToArray] = uint8arrtoarrfn

	uint8arrtostr, _ := v.iso.CompileUnboundScript("(arr) => { return String.fromCharCode(...arr); };", "uint8-array-to-string.js", v8.CompileOptions{})
	uint8arrtostrval, _ := uint8arrtostr.Run(v.ctx)
	uint8arrtostrfn, _ := uint8arrtostrval.AsFunction()
	v.utils[v8FunctionUInt8ArrayToString] = uint8arrtostrfn
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

// agentint.{vmID}.rpc.{namespace}.{workload}.http.{method}
func (v *V8) httpServiceSubject(method string) string {
	return fmt.Sprintf("agentint.%s.rpc.%s.%s.http.%s", v.vmID, v.namespace, v.name, method)
}

// agentint.{vmID}.rpc.{namespace}.{workload}.kv.{method}
func (v *V8) keyValueServiceSubject(method string) string {
	return fmt.Sprintf("agentint.%s.rpc.%s.%s.kv.%s", v.vmID, v.namespace, v.name, method)
}

// agentint.{vmID}.rpc.{namespace}.{workload}.messaging.{method}
func (v *V8) messagingServiceSubject(method string) string {
	return fmt.Sprintf("agentint.%s.rpc.%s.%s.messaging.%s", v.vmID, v.namespace, v.name, method)
}

// agentint.{vmID}.rpc.{namespace}.{workload}.objectstore.{method}
func (v *V8) objectStoreServiceSubject(method string) string {
	return fmt.Sprintf("agentint.%s.rpc.%s.%s.objectstore.%s", v.vmID, v.namespace, v.name, strings.ToLower(method))
}

func (v *V8) newHostServicesTemplate() (*v8.ObjectTemplate, error) {
	hostServices := v8.NewObjectTemplate(v.iso)

	err := hostServices.Set(hostServicesHTTPObjectName, v.newHTTPObjectTemplate())
	if err != nil {
		return nil, err
	}

	err = hostServices.Set(hostServicesKVObjectName, v.newKeyValueObjectTemplate())
	if err != nil {
		return nil, err
	}

	err = hostServices.Set(hostServicesMessagingObjectName, v.newMessagingObjectTemplate())
	if err != nil {
		return nil, err
	}

	err = hostServices.Set(hostServicesObjectStoreObjectName, v.newObjectStoreObjectTemplate())
	if err != nil {
		return nil, err
	}

	return hostServices, nil
}

func (v *V8) newHTTPObjectTemplate() *v8.ObjectTemplate {
	http := v8.NewObjectTemplate(v.iso)

	_ = http.Set(hostServicesHTTPGetFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) == 0 {
			val, _ := v8.NewValue(v.iso, "url is required")
			return v.iso.ThrowException(val)
		}

		_url, err := url.Parse(args[0].String())
		if err != nil {
			_, _ = v.stderr.Write([]byte(fmt.Sprintf("failed to parse url: %s", err.Error())))

			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		msg := nats.NewMsg(v.httpServiceSubject(hostServicesHTTPGetFunctionName))
		msg.Header.Add(agentapi.HttpURLHeader, _url.String())

		resp, err := v.nc.RequestMsg(msg, hostServicesHTTPRequestTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var httpresp *agentapi.HostServicesHTTPResponse
		err = json.Unmarshal(resp.Data, &httpresp)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		if httpresp.Error != nil {
			val, _ := v8.NewValue(v.iso, fmt.Sprintf("failed to complete HTTP request: %s", *httpresp.Error))
			return v.iso.ThrowException(val)
		}

		respobj := v8.NewObjectTemplate(v.iso)
		_ = respobj.Set("status", int32(httpresp.Status))

		if httpresp.Headers != nil {
			var headers map[string][]string
			err = json.Unmarshal(*httpresp.Headers, &headers)
			if err != nil {
				_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to unmarshal response headers for http GET request: %s", err.Error())))

				val, _ := v8.NewValue(v.iso, err.Error())
				return v.iso.ThrowException(val)
			}

			hdrsobj := v8.NewObjectTemplate(v.iso)
			for hdr, hdrval := range headers {
				hdrstr := strings.Join(hdrval, ", ")
				_hdrval, _ := v8.NewValue(v.iso, strings.TrimRightFunc(hdrstr, unicode.IsSpace))
				_ = hdrsobj.Set(hdr, _hdrval)
			}

			_ = respobj.Set("headers", hdrsobj)
		}

		if len(httpresp.Body) > 0 {
			_ = respobj.Set("response", httpresp.Body)
		} else {
			_ = respobj.Set("response", nil)
		}

		_, _ = v.stdout.Write([]byte(fmt.Sprintf("received %d-byte response (%d status) for http GET request", len(httpresp.Body), httpresp.Status)))

		val, err := respobj.NewInstance(v.ctx)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		return val.Value
	}))

	_ = http.Set(hostServicesHTTPPostFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) == 0 {
			val, _ := v8.NewValue(v.iso, "url is required")
			return v.iso.ThrowException(val)
		}

		_url, err := url.Parse(args[0].String())
		if err != nil {
			_, _ = v.stderr.Write([]byte(fmt.Sprintf("failed to parse url: %s", err.Error())))

			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		payload, err := v.marshalValue(args[1])
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		msg := nats.NewMsg(v.httpServiceSubject(hostServicesHTTPPostFunctionName))
		msg.Header.Add(agentapi.HttpURLHeader, _url.String())
		msg.Data = []byte(payload)

		resp, err := v.nc.RequestMsg(msg, hostServicesHTTPRequestTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var httpresp *agentapi.HostServicesHTTPResponse
		err = json.Unmarshal(resp.Data, &httpresp)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		if httpresp.Error != nil {
			val, _ := v8.NewValue(v.iso, fmt.Sprintf("failed to complete HTTP request: %s", *httpresp.Error))
			return v.iso.ThrowException(val)
		}

		respobj := v8.NewObjectTemplate(v.iso)
		_ = respobj.Set("status", int32(httpresp.Status))

		if httpresp.Headers != nil {
			var headers map[string][]string
			err = json.Unmarshal(*httpresp.Headers, &headers)
			if err != nil {
				_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to unmarshal response headers for http POST request: %s", err.Error())))

				val, _ := v8.NewValue(v.iso, err.Error())
				return v.iso.ThrowException(val)
			}

			hdrsobj := v8.NewObjectTemplate(v.iso)
			for hdr, hdrval := range headers {
				hdrstr := strings.Join(hdrval, ", ")
				_hdrval, _ := v8.NewValue(v.iso, strings.TrimRightFunc(hdrstr, unicode.IsSpace))
				_ = hdrsobj.Set(hdr, _hdrval)
			}

			_ = respobj.Set("headers", hdrsobj)
		}

		if len(httpresp.Body) > 0 {
			_ = respobj.Set("response", httpresp.Body)
		} else {
			_ = respobj.Set("response", nil)
		}

		_, _ = v.stdout.Write([]byte(fmt.Sprintf("received %d-byte response (%d status) for http POST request", len(httpresp.Body), httpresp.Status)))

		val, err := respobj.NewInstance(v.ctx)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		return val.Value
	}))

	_ = http.Set(hostServicesHTTPPutFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) == 0 {
			val, _ := v8.NewValue(v.iso, "url is required")
			return v.iso.ThrowException(val)
		}

		_url, err := url.Parse(args[0].String())
		if err != nil {
			_, _ = v.stderr.Write([]byte(fmt.Sprintf("failed to parse url: %s", err.Error())))

			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		payload, err := v.marshalValue(args[1])
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		msg := nats.NewMsg(v.httpServiceSubject(hostServicesHTTPPutFunctionName))
		msg.Header.Add(agentapi.HttpURLHeader, _url.String())
		msg.Data = []byte(payload)

		resp, err := v.nc.RequestMsg(msg, hostServicesHTTPRequestTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var httpresp *agentapi.HostServicesHTTPResponse
		err = json.Unmarshal(resp.Data, &httpresp)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		if httpresp.Error != nil {
			val, _ := v8.NewValue(v.iso, fmt.Sprintf("failed to complete HTTP request: %s", *httpresp.Error))
			return v.iso.ThrowException(val)
		}

		respobj := v8.NewObjectTemplate(v.iso)
		_ = respobj.Set("status", int32(httpresp.Status))

		if httpresp.Headers != nil {
			var headers map[string][]string
			err = json.Unmarshal(*httpresp.Headers, &headers)
			if err != nil {
				_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to unmarshal response headers for http PUT request: %s", err.Error())))

				val, _ := v8.NewValue(v.iso, err.Error())
				return v.iso.ThrowException(val)
			}

			hdrsobj := v8.NewObjectTemplate(v.iso)
			for hdr, hdrval := range headers {
				hdrstr := strings.Join(hdrval, ", ")
				_hdrval, _ := v8.NewValue(v.iso, strings.TrimRightFunc(hdrstr, unicode.IsSpace))
				_ = hdrsobj.Set(hdr, _hdrval)
			}

			_ = respobj.Set("headers", hdrsobj)
		}

		if len(httpresp.Body) > 0 {
			_ = respobj.Set("response", httpresp.Body)
		} else {
			_ = respobj.Set("response", nil)
		}

		_, _ = v.stdout.Write([]byte(fmt.Sprintf("received %d-byte response (%d status) for http PUT request", len(httpresp.Body), httpresp.Status)))

		val, err := respobj.NewInstance(v.ctx)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		return val.Value
	}))

	_ = http.Set(hostServicesHTTPPatchFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) == 0 {
			val, _ := v8.NewValue(v.iso, "url is required")
			return v.iso.ThrowException(val)
		}

		_url, err := url.Parse(args[0].String())
		if err != nil {
			_, _ = v.stderr.Write([]byte(fmt.Sprintf("failed to parse url: %s", err.Error())))

			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		payload, err := v.marshalValue(args[1])
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		msg := nats.NewMsg(v.httpServiceSubject(hostServicesHTTPPatchFunctionName))
		msg.Header.Add(agentapi.HttpURLHeader, _url.String())
		msg.Data = []byte(payload)

		resp, err := v.nc.RequestMsg(msg, hostServicesHTTPRequestTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var httpresp *agentapi.HostServicesHTTPResponse
		err = json.Unmarshal(resp.Data, &httpresp)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		if httpresp.Error != nil {
			val, _ := v8.NewValue(v.iso, fmt.Sprintf("failed to complete HTTP request: %s", *httpresp.Error))
			return v.iso.ThrowException(val)
		}

		respobj := v8.NewObjectTemplate(v.iso)
		_ = respobj.Set("status", int32(httpresp.Status))

		if httpresp.Headers != nil {
			var headers map[string][]string
			err = json.Unmarshal(*httpresp.Headers, &headers)
			if err != nil {
				_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to unmarshal response headers for http PATCH request: %s", err.Error())))

				val, _ := v8.NewValue(v.iso, err.Error())
				return v.iso.ThrowException(val)
			}

			hdrsobj := v8.NewObjectTemplate(v.iso)
			for hdr, hdrval := range headers {
				hdrstr := strings.Join(hdrval, ", ")
				_hdrval, _ := v8.NewValue(v.iso, strings.TrimRightFunc(hdrstr, unicode.IsSpace))
				_ = hdrsobj.Set(hdr, _hdrval)
			}

			_ = respobj.Set("headers", hdrsobj)
		}

		if len(httpresp.Body) > 0 {
			_ = respobj.Set("response", httpresp.Body)
		} else {
			_ = respobj.Set("response", nil)
		}

		_, _ = v.stdout.Write([]byte(fmt.Sprintf("received %d-byte response (%d status) for http PATCH request", len(httpresp.Body), httpresp.Status)))

		val, err := respobj.NewInstance(v.ctx)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		return val.Value
	}))

	_ = http.Set(hostServicesHTTPDeleteFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) == 0 {
			val, _ := v8.NewValue(v.iso, "url is required")
			return v.iso.ThrowException(val)
		}

		_url, err := url.Parse(args[0].String())
		if err != nil {
			_, _ = v.stderr.Write([]byte(fmt.Sprintf("failed to parse url: %s", err.Error())))

			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		msg := nats.NewMsg(v.httpServiceSubject(hostServicesHTTPDeleteFunctionName))
		msg.Header.Add(agentapi.HttpURLHeader, _url.String())

		resp, err := v.nc.RequestMsg(msg, hostServicesHTTPRequestTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var httpresp *agentapi.HostServicesHTTPResponse
		err = json.Unmarshal(resp.Data, &httpresp)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		if httpresp.Error != nil {
			val, _ := v8.NewValue(v.iso, fmt.Sprintf("failed to complete HTTP request: %s", *httpresp.Error))
			return v.iso.ThrowException(val)
		}

		respobj := v8.NewObjectTemplate(v.iso)
		_ = respobj.Set("status", int32(httpresp.Status))

		if httpresp.Headers != nil {
			var headers map[string][]string
			err = json.Unmarshal(*httpresp.Headers, &headers)
			if err != nil {
				_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to unmarshal response headers for http DELETE request: %s", err.Error())))

				val, _ := v8.NewValue(v.iso, err.Error())
				return v.iso.ThrowException(val)
			}

			hdrsobj := v8.NewObjectTemplate(v.iso)
			for hdr, hdrval := range headers {
				hdrstr := strings.Join(hdrval, ", ")
				_hdrval, _ := v8.NewValue(v.iso, strings.TrimRightFunc(hdrstr, unicode.IsSpace))
				_ = hdrsobj.Set(hdr, _hdrval)
			}

			_ = respobj.Set("headers", hdrsobj)
		}

		if len(httpresp.Body) > 0 {
			_ = respobj.Set("response", httpresp.Body)
		} else {
			_ = respobj.Set("response", nil)
		}

		_, _ = v.stdout.Write([]byte(fmt.Sprintf("received %d-byte response (%d status) for http DELETE request", len(httpresp.Body), httpresp.Status)))

		val, err := respobj.NewInstance(v.ctx)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		return val.Value
	}))

	_ = http.Set(hostServicesHTTPHeadFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) == 0 {
			val, _ := v8.NewValue(v.iso, "url is required")
			return v.iso.ThrowException(val)
		}

		_url, err := url.Parse(args[0].String())
		if err != nil {
			_, _ = v.stderr.Write([]byte(fmt.Sprintf("failed to parse url: %s", err.Error())))

			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		msg := nats.NewMsg(v.httpServiceSubject(hostServicesHTTPHeadFunctionName))
		msg.Header.Add(agentapi.HttpURLHeader, _url.String())

		resp, err := v.nc.RequestMsg(msg, hostServicesHTTPRequestTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var httpresp *agentapi.HostServicesHTTPResponse
		err = json.Unmarshal(resp.Data, &httpresp)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		if httpresp.Error != nil {
			val, _ := v8.NewValue(v.iso, fmt.Sprintf("failed to complete HTTP request: %s", *httpresp.Error))
			return v.iso.ThrowException(val)
		}

		respobj := v8.NewObjectTemplate(v.iso)
		_ = respobj.Set("status", int32(httpresp.Status))

		if httpresp.Headers != nil {
			var headers map[string][]string
			err = json.Unmarshal(*httpresp.Headers, &headers)
			if err != nil {
				_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to unmarshal response headers for http HEAD request: %s", err.Error())))

				val, _ := v8.NewValue(v.iso, err.Error())
				return v.iso.ThrowException(val)
			}

			hdrsobj := v8.NewObjectTemplate(v.iso)
			for hdr, hdrval := range headers {
				hdrstr := strings.Join(hdrval, ", ")
				_hdrval, _ := v8.NewValue(v.iso, strings.TrimRightFunc(hdrstr, unicode.IsSpace))
				_ = hdrsobj.Set(hdr, _hdrval)
			}

			_ = respobj.Set("headers", hdrsobj)
		}

		_, _ = v.stdout.Write([]byte(fmt.Sprintf("received response (%d status) for http HEAD request", httpresp.Status)))

		val, err := respobj.NewInstance(v.ctx)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		return val.Value
	}))

	return http
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

		msg := nats.NewMsg(v.keyValueServiceSubject(hostServicesKVGetFunctionName))
		msg.Header.Add(agentapi.KeyValueKeyHeader, key)

		resp, err := v.nc.RequestMsg(msg, hostServicesKVGetTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		val, err := v.toUInt8ArrayValue(resp.Data)
		if err != nil {
			_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to convert raw %d-length []byte to Uint8[]: %s", len(resp.Data), err.Error())))
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

		value, err := v.marshalValue(args[1])
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		msg := nats.NewMsg(v.keyValueServiceSubject(hostServicesKVSetFunctionName))
		msg.Header.Add(agentapi.KeyValueKeyHeader, key)
		msg.Data = value

		resp, err := v.nc.RequestMsg(msg, hostServicesKVSetTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var kvresp *agentapi.HostServicesKeyValueResponse
		err = json.Unmarshal(resp.Data, &kvresp)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		if !*kvresp.Success {
			val, _ := v8.NewValue(v.iso, fmt.Sprintf("failed to set %d-byte value for key: %s", len(value), key))
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

		msg := nats.NewMsg(v.keyValueServiceSubject(hostServicesKVDeleteFunctionName))
		msg.Header.Add(agentapi.KeyValueKeyHeader, key)

		resp, err := v.nc.RequestMsg(msg, hostServicesKVDeleteTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var kvresp *agentapi.HostServicesKeyValueResponse
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
		payload, err := v.marshalValue(args[1])
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		msg := nats.NewMsg(v.messagingServiceSubject(hostServicesMessagingPublishFunctionName))
		msg.Header.Add(agentapi.MessagingSubjectHeader, subject)
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
		payload, err := v.marshalValue(args[1])
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		msg := nats.NewMsg(v.messagingServiceSubject(hostServicesMessagingRequestFunctionName))
		msg.Header.Add(agentapi.MessagingSubjectHeader, subject)
		msg.Data = []byte(payload)

		resp, err := v.nc.RequestMsg(msg, hostServicesMessagingRequestTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		val, err := v.toUInt8ArrayValue(resp.Data)
		if err != nil {
			_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to convert raw %d-length []byte to Uint8[]: %s", len(resp.Data), err.Error())))
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

		payload, err := v.marshalValue(args[1])
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		// construct the requestMany request message
		msg := nats.NewMsg(v.messagingServiceSubject(hostServicesMessagingRequestManyFunctionName))
		msg.Header.Add(agentapi.MessagingSubjectHeader, subject)
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

				respval, err := v.toUInt8ArrayValue(resp.Data)
				if err != nil {
					_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to convert raw %d-length []byte to Uint8[]: %s", len(resp.Data), err.Error())))
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

func (v *V8) newObjectStoreObjectTemplate() *v8.ObjectTemplate {
	objectStore := v8.NewObjectTemplate(v.iso)

	_ = objectStore.Set(hostServicesObjectStoreGetFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) != 1 {
			val, _ := v8.NewValue(v.iso, "name is required")
			return v.iso.ThrowException(val)
		}

		name := args[0].String()

		msg := nats.NewMsg(v.objectStoreServiceSubject(hostServicesObjectStoreGetFunctionName))
		msg.Header.Add(agentapi.ObjectStoreObjectNameHeader, name)

		resp, err := v.nc.RequestMsg(msg, hostServicesObjectStoreGetTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		val, err := v.toUInt8ArrayValue(resp.Data)
		if err != nil {
			_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to convert raw %d-length []byte to Uint8[]: %s", len(resp.Data), err.Error())))
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		return val
	}))

	_ = objectStore.Set(hostServicesObjectStorePutFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) != 2 {
			val, _ := v8.NewValue(v.iso, "name and value are required")
			return v.iso.ThrowException(val)
		}

		name := args[0].String()

		value, err := v.marshalValue(args[1])
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		msg := nats.NewMsg(v.objectStoreServiceSubject(hostServicesObjectStorePutFunctionName))
		msg.Header.Add(agentapi.ObjectStoreObjectNameHeader, name)
		msg.Data = []byte(value)

		resp, err := v.nc.RequestMsg(msg, hostServicesObjectStorePutTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		val, err := v8.JSONParse(v.ctx, string(resp.Data)) // nats.ObjectMeta JSON
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		return val
	}))

	_ = objectStore.Set(hostServicesObjectStoreDeleteFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) != 1 {
			val, _ := v8.NewValue(v.iso, "name is required")
			return v.iso.ThrowException(val)
		}

		name := args[0].String()

		msg := nats.NewMsg(v.objectStoreServiceSubject(hostServicesObjectStoreDeleteFunctionName))
		msg.Header.Add(agentapi.ObjectStoreObjectNameHeader, name)

		resp, err := v.nc.RequestMsg(msg, hostServicesObjectStoreDeleteTimeout)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		var objectStoreResp *agentapi.HostServicesObjectStoreResponse
		err = json.Unmarshal(resp.Data, &objectStoreResp)
		if err != nil {
			val, _ := v8.NewValue(v.iso, err.Error())
			return v.iso.ThrowException(val)
		}

		if !objectStoreResp.Success {
			val, _ := v8.NewValue(v.iso, fmt.Sprintf("failed to delete object: %s", name))
			return v.iso.ThrowException(val)
		}

		return nil
	}))

	_ = objectStore.Set(hostServicesObjectStoreListFunctionName, v8.NewFunctionTemplate(v.iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		req, _ := json.Marshal(map[string]interface{}{})

		resp, err := v.nc.Request(v.objectStoreServiceSubject(hostServicesObjectStoreListFunctionName), req, hostServicesObjectStoreListTimeout)
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

	return objectStore
}

// marshal the given v8 value to an array of bytes that can be sent over the wire
func (v *V8) marshalValue(val *v8.Value) ([]byte, error) {
	if val.IsUint8Array() {
		v, err := v.utils[v8FunctionUInt8ArrayToString].Call(v.ctx.Global(), val)
		if err != nil {
			return nil, err
		}

		return []byte(v.String()), nil
	}

	return nil, fmt.Errorf("failed to marshal v8 value to []byte: %v; only Uint8[] is supported", val)
}

// unmarshal the given []byte value into a native Uint8Array which can be handed back into v8,
func (v *V8) toUInt8ArrayValue(val []byte) (*v8.Value, error) {
	// initialize a v8 value representing the size in bytes of the native Uint8Array to be allocated
	len, err := v8.NewValue(v.iso, uint64(len(val)))
	if err != nil {
		return nil, err
	}

	// initialize a native Uint8Array
	nativeUint8Arr, err := v.utils[v8FunctionUInt8ArrayInit].Call(v.ctx.Global(), len)
	if err != nil {
		return nil, err
	}

	for i, _uint := range val {
		// initialize a v8 value representing the current byte offset in our native Uint8Array
		_i, err := v8.NewValue(v.iso, uint64(i))
		if err != nil {
			_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to cast i index value to uint32: %s", err.Error())))
			return nil, err
		}

		// pack 8 bits into a uint32, as this is needed when initializing a v8.Value
		_val, err := v8.NewValue(v.iso, uint32(_uint))
		if err != nil {
			_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to cast byte to uint32: %s", err.Error())))
			return nil, err
		}

		// write 8 bits to the current byte offset in the native Uint8Array
		nativeUint8Arr, err = v.utils[v8FunctionUInt8ArraySetIdx].Call(v.ctx.Global(), nativeUint8Arr, _i, _val)
		if err != nil {
			_, _ = v.stdout.Write([]byte(fmt.Sprintf("failed to call %s: %s", v8FunctionUInt8ArraySetIdx, err.Error())))
			return nil, err
		}
	}

	return nativeUint8Arr, nil
}

// convenience method to initialize a V8 execution provider
func InitNexExecutionProviderV8(params *agentapi.ExecutionProviderParams) (*V8, error) {
	if params.WorkloadName == nil {
		return nil, errors.New("V8 execution provider requires a workload name parameter")
	}

	if params.TmpFilename == nil {
		return nil, errors.New("V8 execution provider requires a temporary filename parameter")
	}

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
