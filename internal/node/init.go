package nexnode

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	nexmodels "github.com/synadia-io/nex/internal/models"
)

func CmdUp(opts *nexmodels.Options, nodeopts *nexmodels.NodeOptions, ctx context.Context, cancel context.CancelFunc, log *slog.Logger) error {
	node, err := NewNode(opts, nodeopts, ctx, cancel, log)
	if err != nil {
		return fmt.Errorf("failed to initialize node: %s", err)
	}

	go node.Start()

	return nil
}

func CmdPreflight(opts *nexmodels.Options, nodeopts *nexmodels.NodeOptions, ctx context.Context, cancel context.CancelFunc, log *slog.Logger) error {
	if nodeopts.PreflightInit != "" {
		fi, err := os.Stat(nodeopts.ConfigFilepath)
		if os.IsNotExist(err) {
			f, err := os.Create(nodeopts.ConfigFilepath)
			if err != nil {
				return err
			}
			defer f.Close()
			if nodeopts.PreflightInit == "sandbox" {
				_, err = f.WriteString(defaultNodeConfig)
				if err != nil {
					return err
				}
			} else if nodeopts.PreflightInit == "nosandbox" {
				_, err = f.WriteString(defaultNoSandboxNodeConfig)
				if err != nil {
					return err
				}
			}
			log.Debug("Configuration file created.", slog.String("path", nodeopts.ConfigFilepath))
		} else if err != nil || fi.IsDir() {
			return err
		} else {
			log.Warn("Configuration file already exists, skipping init.", slog.String("path", nodeopts.ConfigFilepath))
		}
	}

	config, err := LoadNodeConfiguration(nodeopts.ConfigFilepath)
	if err != nil {
		return fmt.Errorf("failed to load configuration file: %s", err)
	}

	config.ForceDepInstall = nodeopts.ForceDepInstall

	err = CheckPrerequisites(config, false, log)
	if err != nil {
		return fmt.Errorf("preflight checks failed: %s", err)
	}

	return nil
}

const defaultNodeConfig string = `{
    "default_resource_dir":"/tmp/nex",
    "machine_pool_size": 1,
    "internal_node_host":"10.10.10.1",
    "cni": {
        "network_name": "fcnet",
        "interface_name": "veth0",
        "subnet": "10.10.10.0/24"
    },
    "machine_template": {
        "vcpu_count": 1,
        "memsize_mib": 256
    },
    "tags": {
        "simple": "true"
    }
}`

const defaultNoSandboxNodeConfig string = `{
    "default_resource_dir":"/tmp/nex",
    "machine_pool_size": 1,
    "internal_node_host":"10.10.10.1",
    "cni": {
        "network_name": "fcnet",
        "interface_name": "veth0",
        "subnet": "10.10.10.0/24"
    },
    "machine_template": {
        "vcpu_count": 1,
        "memsize_mib": 256
    },
    "tags": {
        "simple": "true"
    },
    "no_sandbox": true
}`
