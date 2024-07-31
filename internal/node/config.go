package nexnode

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	controlapi "github.com/synadia-io/nex/control-api"
	"github.com/synadia-io/nex/internal/models"
)

// Reads the node configuration from the specified configuration file path
func LoadNodeConfiguration(configFilepath string) (*models.NodeConfiguration, error) {
	bytes, err := os.ReadFile(configFilepath)
	if err != nil {
		return nil, err
	}

	config := models.DefaultNodeConfiguration()
	err = json.Unmarshal(bytes, &config)
	if err != nil {
		return nil, err
	}

	if config.AllowDuplicateWorkloads == nil {
		// allow duplicate workloads by default in sandbox mode; disallow by default in no-sandbox mode
		allowDuplicateWorkloads := !config.NoSandbox
		config.AllowDuplicateWorkloads = &allowDuplicateWorkloads
	}

	if len(config.WorkloadTypes) == 0 {
		config.WorkloadTypes = []controlapi.NexWorkload{controlapi.NexWorkloadNative}
	}

	if (strings.EqualFold(runtime.GOOS, "windows") || strings.EqualFold(runtime.GOOS, "darwin")) && !config.NoSandbox {
		return nil, errors.New("host must be configured to run in no sandbox mode")
	}

	if config.KernelFilepath == "" && config.DefaultResourceDir != "" {
		config.KernelFilepath = filepath.Join(config.DefaultResourceDir, "vmlinux")
	} else if config.KernelFilepath == "" && config.DefaultResourceDir == "" {
		return nil, errors.New("invalid kernel file setting")
	}

	if config.RootFsFilepath == "" && config.DefaultResourceDir != "" {
		config.RootFsFilepath = filepath.Join(config.DefaultResourceDir, "rootfs.ext4")
	} else if config.RootFsFilepath == "" && config.DefaultResourceDir == "" {
		return nil, errors.New("invalid rootfs file setting")
	}

	if config.Tags == nil {
		config.Tags = make(map[string]string)
	}

	for _, sub := range models.RequiredTriggerSubjectDenyList {
		if !slices.Contains(config.DenyTriggerSubjects, sub) {
			config.DenyTriggerSubjects = append(config.DenyTriggerSubjects, sub)
		}
	}

	return &config, nil
}
