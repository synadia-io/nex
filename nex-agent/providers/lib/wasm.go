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

// Wasm execution provider implementation
type Wasm struct {
	wasmFile []byte
}

func (e *Wasm) Deploy() error {
	return errors.New("wasm execution provider not yet implemented")
}

func (e *Wasm) Execute(subject string, payload []byte) ([]byte, error) {
	return nil, errors.New("wasm execution provider does not support trigger execution... yet ;)")
}

func (e *Wasm) Validate() error {
	return errors.New("wasm execution provider not yet implemented")
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
	}, nil
}

// if the return slice is missing or empty, that counts as a "no reply"
func (e *Wasm) RunTrigger(subject string, payload []byte) ([]byte, error) {

	ctx := context.Background()

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	outBuf := newStdOutBuf()
	inBuf := newStdInBuf(payload)
	config := wazero.NewModuleConfig().
		WithStdin(inBuf).
		WithStdout(outBuf).
		WithStderr(os.Stderr)

	// Instantiate WASI, which implements system I/O such as console output.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	_, err := r.InstantiateWithConfig(ctx, e.wasmFile, config.WithArgs("nexfunction", subject))
	if err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
			// TODO: log error
			return nil, err
		} else if !ok {
			// TODO: log failure
			return nil, errors.New("failed to execute WASI function")
		}
	} else {
		return outBuf.buf, err
	}
	return nil, errors.New("unknown")
}

type stdOutBuf struct {
	buf []byte
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

func newStdInBuf(input []byte) *stdInBuf {
	return &stdInBuf{
		data: input,
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
