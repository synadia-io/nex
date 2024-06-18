package preflight

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/synadia-io/nex/internal/models"
)

func preflightInit(config *models.NodeConfiguration, _ *slog.Logger) ([]*requirement, PreflightError) {
	if config.NoSandbox {
		required := []*requirement{
			{
				name: "nex-agent", path: config.BinPath, nosandbox: true,
				description: "Nex-agent binary",
				dlUrl:       fmt.Sprintf(nexAgentLinuxURLTemplate, nexLatestVersion, nexLatestVersion),
				shaUrl:      fmt.Sprintf(nexAgentLinuxURLTemplateSHA256, nexLatestVersion, nexLatestVersion),
				iF:          downloadDirect,
			},
		}
		return required, nil
	}
	required := []*requirement{
		{name: "host-local", path: config.CNI.BinPath, nosandbox: false, description: "host-local CNI plugin", iF: downloadTarGz, dlUrl: cniPluginsTarballURL, shaUrl: cniPluginsTarballSHA256, tarHeader: "./host-local"},
		{name: "ptp", path: config.CNI.BinPath, nosandbox: false, description: "ptp CNI plugin", iF: downloadTarGz, dlUrl: cniPluginsTarballURL, shaUrl: cniPluginsTarballSHA256, tarHeader: "./ptp"},
		{name: "tc-redirect-tap", path: config.CNI.BinPath, nosandbox: false, description: "tc-redirect-tap CNI plugin", dlUrl: tcRedirectCNIPluginURL, shaUrl: tcRedirectCNIPluginSHA256, iF: downloadDirect},
		{name: "firecracker", path: config.BinPath, nosandbox: false, description: "Firecracker VM binary", iF: downloadTarGz, dlUrl: firecrackerTarballURL, shaUrl: firecrackerTarballSHA256, tarHeader: firecrackerTarHeaderString},
		{name: *config.CNI.NetworkName + ".conflist", path: []string{"/etc/cni/conf.d"}, nosandbox: false, description: "CNI Configuration", iF: writeCniConf},
		{name: filepath.Base(config.KernelFilepath), path: []string{filepath.Dir(config.KernelFilepath)}, nosandbox: false, description: "VMLinux Kernel", dlUrl: vmLinuxKernelURL, shaUrl: vmLinuxKernelSHA256, iF: downloadDirect},
		{name: filepath.Base(config.RootFsFilepath), path: []string{filepath.Dir(config.RootFsFilepath)}, nosandbox: false, description: "Root Filesystem Template", iF: downloadGz, dlUrl: fmt.Sprintf(rootfsGzipURLTemplate, nexLatestVersion), shaUrl: fmt.Sprintf(rootfsGzipSHA256Template, nexLatestVersion)},
	}

	return required, nil
}
