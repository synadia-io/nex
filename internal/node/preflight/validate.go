package preflight

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"

	"github.com/synadia-io/nex/internal/models"
)

type BinaryVerify struct {
	BinName string
	Error   PreflightError
}

type PreflightError error

var (
	ErrNexAgentNotFound            = errors.New("nex-agent binary not found")
	ErrFirecrackerNotFound         = errors.New("firecracker binary not found")
	ErrHostLocalPluginNotFound     = errors.New("host-local binary not found")
	ErrPtpPluginNotFound           = errors.New("ptp binary not found")
	ErrTcRedirectTapPluginNotFound = errors.New("tc-redirect-tap binary not found")
	ErrRootFsNotFound              = errors.New("rootfs file not found")
	ErrVmlinuxNotFound             = errors.New("vmlinux file not found")
	ErrCNIConfigNotFound           = errors.New("cni config file not found")

	ErrNoSandboxRequired           = errors.New("no sandbox required for os")
	ErrFailedToDownloadNexAgent    = errors.New("failed to download nex-agent")
	ErrFailedToDownloadNexAgentSha = errors.New("failed to download nex-agent")
	ErrFailedToCreateTempFile      = errors.New("failed to create temp binary file")
	ErrFailedToWriteTempFile       = errors.New("failed to write temp binary file")
	ErrFailedToCalculateSha256     = errors.New("failed to calculate sha256")
	ErrSha256Mismatch              = errors.New("sha256 mismatch")

	binVerify = []BinaryVerify{
		{BinName: "firecracker", Error: ErrFirecrackerNotFound},
		{BinName: "host-local", Error: ErrHostLocalPluginNotFound},
		{BinName: "ptp", Error: ErrPtpPluginNotFound},
		{BinName: "tc-redirect-tap", Error: ErrTcRedirectTapPluginNotFound},
	}
)

func Validate(config *models.NodeConfiguration, logger *slog.Logger) PreflightError {
	var errs PreflightError
	if config.NoSandbox {
		nexAgentPath, err := exec.LookPath("nex-agent")
		if err != nil {
			errs = errors.Join(errs, ErrNexAgentNotFound)
		} else {
			logger.Debug("nex-agent binary found", slog.String("path", nexAgentPath))
		}
		return errs
	}

	for _, bin := range binVerify {
		binPath, err := exec.LookPath(bin.BinName)
		if err != nil {
			errs = errors.Join(errs, bin.Error)
		} else {
			logger.Debug(bin.BinName+" binary found", slog.String("path", binPath))
		}
	}

	_, err := os.Stat(config.RootFsFilepath)
	if errors.Is(err, os.ErrNotExist) {
		errs = errors.Join(errs, ErrRootFsNotFound)
	} else {
		logger.Debug("rootfs file found", slog.String("path", config.RootFsFilepath))
	}

	_, err = os.Stat(config.KernelFilepath)
	if errors.Is(err, os.ErrNotExist) {
		errs = errors.Join(errs, ErrVmlinuxNotFound)
	} else {
		logger.Debug("vmlinux file found", slog.String("path", config.KernelFilepath))
	}

	_, err = os.Stat("/etc/cni/conf.s" + *config.CNI.NetworkName + ".conflist")
	if errors.Is(err, os.ErrNotExist) {
		errs = errors.Join(errs, ErrCNIConfigNotFound)
	} else {
		logger.Debug("cni config file found", slog.String("path", "/etc/cni/conf.d/"+*config.CNI.NetworkName+".conflist"))
	}

	if errs != nil {
		logger.Warn("⛔ preflight validation failed")
		logger.Debug("\tPreflight requirement missing", slog.Any("err", err))
	}

	return errs
}
