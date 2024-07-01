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
	Path    []string
}

type PreflightError error

var (
	ErrBinaryNotFound      = errors.New("binary not found")
	ErrBinaryNotExecutable = errors.New("binary not executable")
	ErrRootFsNotFound      = errors.New("rootfs file not found")
	ErrVmlinuxNotFound     = errors.New("vmlinux file not found")
	ErrCNIConfigNotFound   = errors.New("cni config file not found")

	ErrNoSandboxRequired                 = errors.New("no sandbox required for os")
	ErrFailedToDetermineLatestNexVersion = errors.New("failed to determine latest nex version")
	ErrFailedToSatifyDependency          = errors.New("failed to satisfy dependency")
	ErrFailedToDownload                  = errors.New("failed to download file")
	ErrFailedToDownloadKnownGoodSha      = errors.New("failed to download known good shasum")
	ErrFailedToCreateFile                = errors.New("failed to create new binary file")
	ErrFailedToWriteTempFile             = errors.New("failed to write temp binary file")
	ErrFailedToCalculateSha256           = errors.New("failed to calculate sha256")
	ErrSha256Mismatch                    = errors.New("sha256 mismatch")
	ErrFailedToUncompress                = errors.New("failed to uncompress file")
	ErrUserCanceledPreflight             = errors.New("user canceled preflight")
)

func verifyPath(binary string, path []string, logger *slog.Logger) (string, PreflightError) {
	if len(path) == 0 {
		e, err := exec.LookPath(binary)
		if err != nil {
			logger.Debug(binary + " binary NOT found")
			return "", ErrBinaryNotFound
		}
		return e, nil
	}

	for _, p := range path {
		e := filepath.Join(p, binary)
		f, err := os.Stat(e)
		if err != nil {
			continue
		}

		if f.Mode()&0100 == 0 {
			logger.Debug(binary+" binary NOT executable", slog.String("path", e))
			return "", ErrBinaryNotExecutable
		} else {
			logger.Debug(binary+" binary found", slog.String("path", e))
			return e, nil
		}
	}

	logger.Debug(binary + " binary NOT found")
	return "", ErrBinaryNotFound
}

func Validate(config *models.NodeConfiguration, logger *slog.Logger) PreflightError {
	var errs PreflightError
	var binVerify []BinaryVerify
	if config.NoSandbox {
		binVerify = []BinaryVerify{
			{BinName: "nex-agent", Path: config.BinPath},
		}
	} else {
		binVerify = []BinaryVerify{
			{BinName: "firecracker", Path: config.BinPath},
			{BinName: "host-local", Path: config.CNI.BinPath},
			{BinName: "ptp", Path: config.CNI.BinPath},
			{BinName: "tc-redirect-tap", Path: config.CNI.BinPath},
		}
	}

	for _, bin := range binVerify {
		_, err := verifyPath(bin.BinName, bin.Path, logger)
		if err != nil {
			errs = errors.Join(errs, err)
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
