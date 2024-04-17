package nexnode

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

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

	// TODO: should we validate the configuration here
	// if !config.Validate() {
	// 	return nil, errors.New("invalid configuration provided: " + err.Error())
	// }

	if len(config.WorkloadTypes) == 0 {
		config.WorkloadTypes = models.DefaultWorkloadTypes
	}

	// TODO-- audit for *string
	if config.KernelFilepath == "" && config.DefaultResourceDir != "" {
		config.KernelFilepath = filepath.Join(config.DefaultResourceDir, "vmlinux")
	} else if config.KernelFilepath == "" && config.DefaultResourceDir == "" {
		return nil, errors.New("invalid kernel file setting")
	}

	// TODO-- audit for *string
	if config.RootFsFilepath == "" && config.DefaultResourceDir != "" {
		config.RootFsFilepath = filepath.Join(config.DefaultResourceDir, "rootfs.ext4")
	} else if config.RootFsFilepath == "" && config.DefaultResourceDir == "" {
		return nil, errors.New("invalid rootfs file setting")
	}

	if config.Tags == nil {
		config.Tags = make(map[string]string)
	}

	return &config, nil
}
