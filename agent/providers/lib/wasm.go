package lib

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// Wasm execution provider implementation
type Wasm struct {
	vmID          string
	wasmFile      []byte
	env           map[string]string
	runtime       wazero.Runtime
	runtimeConfig wazero.ModuleConfig
	module        wazero.CompiledModule

	fail chan bool
	run  chan bool
	exit chan int

	nc *nats.Conn // agent NATS connection
}

func (e *Wasm) Deploy() error {
	subject := fmt.Sprintf("agentint.%s.trigger", e.vmID)
	_, err := e.nc.Subscribe(subject, func(msg *nats.Msg) {
		ctx := otel.GetTextMapPropagator().Extract(context.Background(), propagation.HeaderCarrier(msg.Header))
		ctx = context.WithValue(ctx, agentapi.NexTriggerSubject, subject)

		val, err := e.Execute(ctx, msg.Data)
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

	e.run <- true
	return nil
}

func (e *Wasm) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	var subject string
	sub, ok := ctx.Value(agentapi.NexTriggerSubject).(string)
	if ok {
		subject = sub
	} else {
		return nil, errors.New("failed to execute WASI function; no trigger subject provided in context")
	}

	out := newStdOutBuf()
	in := newStdInBuf()
	in.Reset(payload)

	// clone runtimeConfig for each execution
	cfg := e.runtimeConfig.
		WithStdin(in).
		WithStdout(out).
		WithArgs("nexfunction", subject)

	_, err := e.runtime.InstantiateModule(ctx, e.module, cfg)
	if err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			// TODO: log error
			return nil, err
		} else if !ok {
			// TODO: log failure
			return nil, errors.New("failed to execute WASI function")
		}
	} else {
		return out.buf, err
	}

	return nil, errors.New("unknown")
}

func (e *Wasm) Undeploy() error {
	// We shouldn't have to do anything here since the wasm "owns" no resources
	return nil
}

func (e *Wasm) Validate() error {
	ctx := context.Background()
	e.runtime = wazero.NewRuntime(ctx)
	e.runtimeConfig = wazero.NewModuleConfig().
		WithStderr(os.Stderr)

	for key, val := range e.env {
		e.runtimeConfig = e.runtimeConfig.WithEnv(key, val)
	}

	var err error

	// Instantiate WASI, which implements system I/O such as console output.
	wasimod, err := wasi_snapshot_preview1.NewBuilder(e.runtime).Compile(ctx)
	if err != nil {
		return fmt.Errorf("failed to compile module: %s", err)
	}

	_, err = e.runtime.InstantiateModule(ctx, wasimod, e.runtimeConfig)
	if err != nil {
		return fmt.Errorf("failed to compile wasi_snapshot_preview1 module: %s", err)
	}

	e.module, err = e.runtime.CompileModule(ctx, e.wasmFile)
	if err != nil {
		return fmt.Errorf("failed to compile module: %s", err)
	}

	return nil
}

// InitNexExecutionProviderWasm convenience method to initialize a Wasm execution provider
func InitNexExecutionProviderWasm(params *agentapi.ExecutionProviderParams) (*Wasm, error) {
	if params.WorkloadName == nil {
		return nil, errors.New("wasm execution provider requires a workload name parameter")
	}

	if params.TmpFilename == nil {
		return nil, errors.New("wasm execution provider requires a temporary filename parameter")
	}

	file, err := os.Open(*params.TmpFilename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return &Wasm{
		vmID:     params.VmID,
		wasmFile: bytes,
		env:      params.Environment,

		fail: params.Fail,
		run:  params.Run,
		exit: params.Exit,

		nc: params.NATSConn,
	}, nil
}

type stdOutBuf struct {
	buf []byte
}

func (o *stdOutBuf) Reset() {
	o.buf = o.buf[:0]
}

func newStdOutBuf() *stdOutBuf {
	return &stdOutBuf{
		buf: make([]byte, 0, 1024),
	}
}

func (i *stdOutBuf) Write(p []byte) (n int, err error) {
	i.buf = p

	return len(p), nil
}

type stdInBuf struct {
	data      []byte
	readIndex int64
}

func (i *stdInBuf) Reset(input []byte) {
	i.readIndex = 0
	i.data = input
}

func newStdInBuf() *stdInBuf {
	return &stdInBuf{
		data: nil,
	}
}

func (r *stdInBuf) Read(p []byte) (n int, err error) {
	if r.readIndex >= int64(len(r.data)) {
		err = io.EOF
		return
	}

	n = copy(p, r.data[r.readIndex:])
	r.readIndex += int64(n)
	return
}
