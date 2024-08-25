package providers

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/synadia-io/nex/agent/providers/lib"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

type PluginExecutable struct {
	inner  *lib.NativeExecutable
	params *agentapi.ExecutionProviderParams
}

// NOTE: executable plugin "runner providers" are actually just native executables that
// take the path of the artifact on argv... so we just create a wrapper around the existing
// native runner for plugins
func InitNexExecutionProviderPlugin(params *agentapi.ExecutionProviderParams, md *agentapi.MachineMetadata) (*PluginExecutable, error) {
	err := adjustParamsForPlugin(params)
	if err != nil {
		fmt.Printf("Failed to adjust provider parameters for plugin: %s\n", err)
		return nil, err
	}
	inner, err := lib.InitNexExecutionProviderNative(params)
	if err != nil {
		return nil, err
	}
	return &PluginExecutable{
		inner,
		params,
	}, nil
}

// this shouldn't get called unless sandbox mode is enabled, in which case
// it'll validate that the plugin runner is a 64-bit elf, which is what we want
func (p *PluginExecutable) Validate() error {
	return p.inner.Validate()
}

func (p *PluginExecutable) Name() string {
	return fmt.Sprintf("Plugin Provider - %s", p.params.WorkloadType)
}

func (p *PluginExecutable) Deploy() (err error) {
	return p.inner.Deploy()
}

func (p *PluginExecutable) Undeploy() error {
	return p.inner.Undeploy()
}

func adjustParamsForPlugin(params *agentapi.ExecutionProviderParams) error {
	var pluginExecutablePath string
	var err error

	artifactPath := *params.TmpFilename
	defer func() {
		if r := recover(); r != nil {
			err = errors.Join(err, fmt.Errorf("plugin loader recovered from panic, attempting load of %s", pluginExecutablePath))
		}
	}()

	if params.PluginPath == nil {
		err = errors.New("invalid execution provider specified")
		logError(params.Stderr, fmt.Sprintf("invalid execution provider specified: %s", string(params.WorkloadType)))
		return err
	}
	if params.Namespace == nil {
		err = errors.New("workload needs a namespace to execute")
		logError(params.Stderr, "workload needs a namespace to execute")
		return err
	}
	if params.WorkloadName == nil {
		err = errors.New("workload needs a name to execute")
		logError(params.Stderr, "workload needs a name to execute")
		return err
	}

	// This will end up with something like `./plugins/wasm`
	pluginExecutablePath = path.Join(*params.PluginPath, string(params.WorkloadType))

	if _, err = os.Stat(pluginExecutablePath); errors.Is(err, os.ErrNotExist) {
		// path does not exist
		err = fmt.Errorf("plugin path was specified but the file (%s) does not exist",
			pluginExecutablePath)
		return err
	} else if err != nil {
		return err
	}

	// the tempfilename we get at the start of the func is the path of the artifact. Here we need
	// to make tmpfilename the path of the plugin, and pass the path of
	// the artifact as the last parameter.
	params.TmpFilename = &pluginExecutablePath
	params.Argv = []string{
		*params.Namespace,
		*params.WorkloadName,
		artifactPath,
	}
	return nil
}

func logError(w io.Writer, msg string) {
	fmt.Println(msg)
	_, _ = w.Write([]byte(msg))
}
