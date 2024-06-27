package preflight

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/synadia-io/nex/internal/models"
)

type BinaryVerify struct {
	BinName string
}

type PreflightError error

var (
	ErrBinaryNotFound      = errors.New("nex-agent binary not found")
	ErrBinaryNotExecutable = errors.New("nex-agent binary not executable")
	ErrRootFsNotFound      = errors.New("rootfs file not found")
	ErrVmlinuxNotFound     = errors.New("vmlinux file not found")
	ErrCNIConfigNotFound   = errors.New("cni config file not found")

	ErrNoSandboxRequired            = errors.New("no sandbox required for os")
	ErrFailedToSatifyDependency     = errors.New("failed to satisfy dependency")
	ErrFailedToDownload             = errors.New("failed to download file")
	ErrFailedToDownloadKnownGoodSha = errors.New("failed to download known good shasum")
	ErrFailedToCreateFile           = errors.New("failed to create new binary file")
	ErrFailedToWriteTempFile        = errors.New("failed to write temp binary file")
	ErrFailedToCalculateSha256      = errors.New("failed to calculate sha256")
	ErrSha256Mismatch               = errors.New("sha256 mismatch")
	ErrFailedToUncompress           = errors.New("failed to uncompress file")
	ErrUserCanceledPreflight        = errors.New("user canceled preflight")

	binVerify = []BinaryVerify{
		{BinName: "firecracker"},
		{BinName: "host-local"},
		{BinName: "ptp"},
		{BinName: "tc-redirect-tap"},
	}
)

func Validate(config *models.NodeConfiguration, logger *slog.Logger) PreflightError {
	var errs PreflightError
	if config.NoSandbox {

		if len(config.BinPath) > 0 {
			for _, p := range config.BinPath {
				e := filepath.Join(p, "nex-agent")
				f, err := os.Stat(e)
				if err != nil {
					logger.Debug("nex-agent binary NOT found", slog.String("path", e))
					errs = errors.Join(errs, ErrBinaryNotFound)
				} else if f.Mode()&0100 == 0 {
					logger.Debug("nex-agent binary NOT executable", slog.String("path", e))
					errs = errors.Join(errs, ErrBinaryNotExecutable)
				} else {
					logger.Debug("nex-agent binary found", slog.String("path", e))
				}
			}
		} else {
			nexAgentPath, err := exec.LookPath("nex-agent")
			if err != nil {
				logger.Debug("nex-agent binary NOT found", slog.String("path", nexAgentPath))
				errs = errors.Join(errs, ErrBinaryNotFound)
			} else {
				logger.Debug("nex-agent binary found", slog.String("path", nexAgentPath))
			}
		}
		return errs
	}

	for _, bin := range binVerify {
		if len(config.BinPath) > 0 && bin.BinName == "firecracker" {
			for _, p := range config.BinPath {
				e := filepath.Join(p, bin.BinName)
				f, err := os.Stat(e)
				if err != nil {
					logger.Debug(bin.BinName+" binary NOT found", slog.String("path", e))
					errs = errors.Join(errs, ErrBinaryNotFound)
				} else if f.Mode()&0100 == 0 {
					logger.Debug(bin.BinName+" binary NOT executable", slog.String("path", e))
					errs = errors.Join(errs, ErrBinaryNotExecutable)
				} else {
					logger.Debug(bin.BinName+" binary found", slog.String("path", e))
				}
			}
		} else if len(config.CNI.BinPath) > 0 && bin.BinName != "firecracker" {
			for _, p := range config.CNI.BinPath {
				e := filepath.Join(p, bin.BinName)
				f, err := os.Stat(e)
				if err != nil {
					logger.Debug(bin.BinName+" binary NOT found", slog.String("path", e))
					errs = errors.Join(errs, ErrBinaryNotFound)
				} else if f.Mode()&0100 == 0 {
					logger.Debug(bin.BinName+" binary NOT executable", slog.String("path", e))
					errs = errors.Join(errs, ErrBinaryNotExecutable)
				} else {
					logger.Debug(bin.BinName+" binary found", slog.String("path", e))
				}
			}
		}
		binPath, err := exec.LookPath(bin.BinName)
		if err != nil {
			logger.Debug(bin.BinName+" binary NOT found", slog.String("path", binPath))
			errs = errors.Join(errs, ErrBinaryNotFound)
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

	_, err = os.Stat(filepath.Join("/etc/cni/conf.d", *config.CNI.NetworkName+".conflist"))
	if errors.Is(err, os.ErrNotExist) {
		errs = errors.Join(errs, ErrCNIConfigNotFound)
	} else {
		logger.Debug("cni config file found", slog.String("path", "/etc/cni/conf.d/"+*config.CNI.NetworkName+".conflist"))
	}

	if errs != nil {
		logger.Warn("⛔ preflight validation failed")
		logger.Debug("\tPreflight requirement missing", slog.Any("err", errs))
	}

	return errs
}
