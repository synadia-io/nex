package lib

import (
	"context"
	"errors"
	"io"
	"os"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// TODO: support environment variables

// Wasm execution provider implementation
type Wasm struct {
	wasmFile      []byte
	env           map[string]string
	runtime       wazero.Runtime
	runtimeConfig wazero.ModuleConfig
	inBuf         *stdInBuf
	outBuf        *stdOutBuf
}

func (e *Wasm) Deploy() error {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	e.runtime = r

	config := wazero.NewModuleConfig().
		WithStdin(e.inBuf).
		WithStdout(e.outBuf).
		WithStderr(os.Stderr)

	for key, val := range e.env {
		config = config.WithEnv(key, val)
	}

	e.runtimeConfig = config

	return nil
}

func (e *Wasm) Execute(subject string, payload []byte) ([]byte, error) {
	return e.runTrigger(subject, payload)
}

func (e *Wasm) Validate() error {
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
		wasmFile: bytes,
		env:      params.Environment,
		outBuf:   newStdOutBuf(),
		inBuf:    newStdInBuf(),
	}, nil
}

// if the return slice is missing or empty, that counts as a "no reply"
func (e *Wasm) runTrigger(subject string, payload []byte) ([]byte, error) {

	ctx := context.Background()

	e.outBuf.Reset()
	e.inBuf.Reset(payload)

	// Instantiate WASI, which implements system I/O such as console output.
	wasi_snapshot_preview1.MustInstantiate(ctx, e.runtime)
	_, err := e.runtime.InstantiateWithConfig(ctx, e.wasmFile, e.runtimeConfig.WithArgs("nexfunction", subject))
	if err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			// TODO: log error
			return nil, err
		} else if !ok {
			// TODO: log failure
			return nil, errors.New("failed to execute WASI function")
		}
	} else {
		return e.outBuf.buf, err
	}
	return nil, errors.New("unknown")
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
