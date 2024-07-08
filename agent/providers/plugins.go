package providers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"plugin"
	"runtime"

	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

type workloadPlugin struct {
	plugin   *plugin.Plugin
	provider ExecutionProvider
}

func MaybeLoadPluginProvider(params *agentapi.ExecutionProviderParams) (wp *workloadPlugin, err error) {
	var actualPath string
	defer func() {
		if r := recover(); r != nil {
			err = errors.Join(err, fmt.Errorf("Plugin loader recovered from panic, attempting load of %s", actualPath))
		}
	}()

	if params.PluginPath == nil {
		err = errors.New("invalid execution provider specified")
		logError(params.Stderr, fmt.Sprintf("invalid execution provider specified: %s", string(params.WorkloadType)))
		return
	}

	// This will end up with something like `./plugins/noop.so`
	actualPath = path.Join(*params.PluginPath, string(params.WorkloadType))
	slog.Debug("Attempting provider plugin load", slog.String("path", actualPath))
	switch runtime.GOOS {
	case "windows":
		err = errors.New("windows agents do not support workload provider plugins")
		return
	case "linux":
		actualPath = actualPath + ".so"
	case "darwin":
		actualPath = actualPath + ".dylib"
	}

	if _, err = os.Stat(actualPath); errors.Is(err, os.ErrNotExist) {
		// path does not exist
		err = fmt.Errorf("plugin path was specified but the file (%s) does not exist", actualPath)
		return
	}

	plug, err := plugin.Open(actualPath)
	if err != nil {
		logError(params.Stderr, fmt.Sprintf("failed to open plugin: %s", err.Error()))
		return
	}

	symPlugin, err := plug.Lookup("ExecutionProvider")
	if err != nil {
		logError(params.Stderr, fmt.Sprintf("failed to lookup plugin symbol: %s", err.Error()))
		return
	}

	var provider ExecutionProvider
	provider, ok := symPlugin.(ExecutionProvider)
	if !ok {
		err = errors.New("unexpected type from module symbol 'ExecutionProvider'")
		logError(params.Stderr, "failed to typecast plugin")
		return
	}

	wp = &workloadPlugin{
		plugin:   plug,
		provider: provider,
	}

	return

}

func (w *workloadPlugin) Name() string {
	return w.provider.Name()
}

func (w *workloadPlugin) Deploy() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("deploy recovered from panic")
		}
	}()

	return nil
}

func (w *workloadPlugin) Undeploy() error {
	return nil
}

func (w *workloadPlugin) Execute(ctx context.Context, payload []byte) ([]byte, error) {
	return nil, errors.New("Native execution provider does not support execution via trigger subjects")
}

// Validate the underlying artifact to be a 64-bit linux native ELF
// binary that is statically-linked
func (w *workloadPlugin) Validate() error {
	return nil
}

func logError(w io.Writer, msg string) {
	fmt.Println(msg)
	_, _ = w.Write([]byte(msg))
}
