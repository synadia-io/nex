package preflight

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/synadia-io/nex/internal/models"
	"github.com/synadia-io/nex/internal/node/templates"
)

type requirement struct {
	name        string
	path        []string
	description string
	nosandbox   bool
	satisfied   bool
	dlUrl       string
	shaUrl      string
	tarHeader   string
	iF          func(*requirement, bool, *models.NodeConfiguration, *slog.Logger) PreflightError
}

func Preflight(ctx context.Context, config *models.NodeConfiguration, logger *slog.Logger) PreflightError {
	nexVer := nexLatestVersion(ctx, logger)
	if nexVer == "" {
		logger.Warn("failed to determine latest version of nex")
	} else {
		logger.Debug("using nex version", slog.String("version", nexVer))
	}

	required, err := preflightInit(nexVer, config, logger)
	if errors.Is(err, ErrNoSandboxRequired) {
		logger.Error("Host must be configured to run in no-sandbox mode")
		return err
	}
	if err != nil {
		return err
	}

	var sb strings.Builder

	for _, r := range required {
		for di, dir := range r.path {
			if r.satisfied {
				continue
			}
			if r.nosandbox == config.NoSandbox {
				if di == 0 && config.PreflightVerbose {
					sb.WriteString(fmt.Sprintf("Validating - %s\n", magenta(r.description)))
				}

				if dir != "" && config.PreflightVerbose {
					sb.WriteString(fmt.Sprintf("\t  🔎 Searching - %s \n", cyan(dir)))
				}

				path := filepath.Join(dir, r.name)

				if _, err := os.Stat(path); err == nil {
					r.satisfied = true
					sb.WriteString(fmt.Sprintf("✅ Dependency Satisfied - %s [%s]\n", green(filepath.Join(dir, r.name)), cyan(r.description)))
				}
			} else {
				// Satisfied by not being required
				r.satisfied = true
			}
		}
		if !r.satisfied {
			sb.WriteString(fmt.Sprintf("⛔ Dependency Missing - %s\n", cyan(r.description)))
		}
	}

	fmt.Print(sb.String())
	if config.PreflightCheck || nexVer == "" {
		return nil
	}

	for _, r := range required {
		if r.satisfied && !config.ForceDepInstall {
			continue
		}

		if !config.ForceDepInstall {
			err := checkContinue(fmt.Sprintf("You are missing required dependencies for [%s], do you want to install?", red(r.description)))
			if errors.Is(err, ErrUserCanceledPreflight) {
				return err
			}
		}

		err := os.MkdirAll(r.path[0], 0755)
		if err != nil {
			return err
		}

		if r.iF != nil {
			err := r.iF(r, config.PreflightVerify, config, logger)
			if err != nil {
				logger.Debug("Failed to run initialize function", slog.String("step", r.description), slog.Any("err", err))
				return err
			}
		}
	}
	return nil
}

func downloadDirect(r *requirement, validate bool, config *models.NodeConfiguration, logger *slog.Logger) PreflightError {
	var errs PreflightError

	respBin, err := http.Get(r.dlUrl)
	if err != nil || respBin.StatusCode != 200 {
		errs = errors.Join(errs, ErrFailedToDownload)
	}
	defer respBin.Body.Close()

	var hashBuff bytes.Buffer
	dl_raw := io.TeeReader(respBin.Body, &hashBuff)

	savePath := filepath.Join(r.path[0], r.name)
	artifact, err := os.Create(savePath + ".new")
	if err != nil {
		logger.Debug("failed to create temp save file", slog.String("name", r.name), slog.String("path", savePath+".new"))
		errs = errors.Join(errs, ErrFailedToCreateFile)
	}
	defer os.Remove(savePath + ".new")

	dl, err := io.ReadAll(dl_raw)
	if err != nil {
		logger.Debug("failed to read downloaded file", slog.String("name", r.name), slog.String("path", savePath+".new"))
		errs = errors.Join(errs, ErrFailedToDownload)
	}
	_, err = artifact.Write(dl)
	if err != nil {
		logger.Debug("failed to write temp file", slog.String("path", savePath+".new"))
		errs = errors.Join(errs, ErrFailedToWriteTempFile)
	}
	_ = artifact.Close()

	if validate && r.shaUrl != "" {
		respSha, err := http.Get(r.shaUrl)
		if err != nil || respSha.StatusCode != 200 {
			logger.Debug("failed to download sha", slog.String("artifact", r.name), slog.Any("err", err))
			errs = errors.Join(errs, ErrFailedToDownloadKnownGoodSha)
			err := checkContinue("Failed to download SHA file for verification; continue?")
			if errors.Is(err, ErrUserCanceledPreflight) {
				return errs
			}
		}
		defer respSha.Body.Close()

		dlSha, err := io.ReadAll(respSha.Body)
		if err != nil {
			logger.Debug("failed to read sha response body", slog.String("artifact", r.name), slog.Any("err", err))
			errs = errors.Join(errs, err)
			err := checkContinue("Failed to download SHA file for verification; continue?")
			if errors.Is(err, ErrUserCanceledPreflight) {
				return errs
			}
		}

		h := sha256.New()
		if _, err := io.Copy(h, &hashBuff); err != nil {
			logger.Debug("failed to calculate sha256", slog.String("artifact", r.name), slog.Any("err", err))
			errs = errors.Join(errs, ErrFailedToCalculateSha256)
			err := checkContinue("Failed to calculate shasum to verify downloaded file; continue?")
			if errors.Is(err, ErrUserCanceledPreflight) {
				return errs
			}
		}

		shasum := hex.EncodeToString(h.Sum(nil))
		if shasum != strings.TrimSpace(string(dlSha)) {
			logger.Debug("sha256 mismatch", slog.String("expected", string(dlSha)), slog.String("actual", shasum), slog.String("artifact", r.name))
			errs = errors.Join(errs, ErrSha256Mismatch)
			err := checkContinue("Downloaded file shasum does not match provided shasum; continue?")
			if errors.Is(err, ErrUserCanceledPreflight) {
				return errs
			}
		}
	}
	if validate && r.shaUrl == "" {
		logger.Debug("validate is true but shaUrl is empty", slog.String("artifact", r.name))
		err := checkContinue("Validate set to true but no shasum provided for comparision; continue?")
		if errors.Is(err, ErrUserCanceledPreflight) {
			return errs
		}
	}

	err = os.Rename(savePath+".new", savePath)
	if err != nil {
		logger.Debug("failed to rename artifact", slog.String("from", savePath+".new"), slog.String("to", savePath))
		errs = errors.Join(errs, err)
		return errs
	}

	err = os.Chmod(savePath, 0755)
	if err != nil {
		logger.Debug("Failed to update binary permissions", slog.Any("err", err), slog.String("artifact", r.name))
		errs = errors.Join(errs, err)
	}

	logger.Debug("binary successfully installed", slog.String("path", savePath), slog.String("download_url", r.dlUrl))
	return errs
}

func downloadTarGz(r *requirement, validate bool, _ *models.NodeConfiguration, logger *slog.Logger) PreflightError {
	var errs PreflightError

	var rawTar *tar.Reader
	var err error

	if validate && r.shaUrl == "" {
		logger.Debug("validate is true but shaUrl is empty", slog.String("artifact", r.name))
		err := checkContinue("Validate set to true but no shasum provided for comparision; continue?")
		if errors.Is(err, ErrUserCanceledPreflight) {
			return errs
		}
	}

	if !validate {
		r.shaUrl = ""
	}
	rawTar, _, err = decompressAndValidateTarFromURL(r.dlUrl, r.shaUrl)
	if errors.Is(err, ErrSha256Mismatch) || errors.Is(err, ErrFailedToDownloadKnownGoodSha) {
		logger.Debug("failed to verify tarball", slog.String("artifact", r.name))
		err := checkContinue("Validate set to true but no shasum provided for comparision; continue?")
		if errors.Is(err, ErrUserCanceledPreflight) {
			return errs
		}
	}
	if err != nil {
		logger.Debug("failed to decompress tarball", slog.String("artifact", r.name), slog.Any("err", err))
		return ErrFailedToSatifyDependency
	}

	for {
		header, err := rawTar.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Debug("failed to read header in artifact tar", slog.Any("artifact", r.name), slog.Any("err", err))
			return ErrFailedToSatifyDependency
		}

		if header.Name == r.tarHeader {
			outFile, err := os.Create(filepath.Join(r.path[0], r.name))
			if err != nil {
				logger.Debug("failed to create artifact binary", slog.Any("artifact", r.name), slog.Any("err", err))
				return ErrFailedToSatifyDependency
			}

			_, err = io.Copy(outFile, rawTar)
			if err != nil {
				logger.Debug("failed to copy artifact binary", slog.Any("artifact", r.name), slog.Any("err", err))
				return ErrFailedToSatifyDependency
			}
			outFile.Close()
			err = os.Chmod(outFile.Name(), 0755)
			if err != nil {
				logger.Debug("failed to change permissions on artifact binary", slog.Any("artifact", r.name), slog.Any("err", err))
				return ErrFailedToSatifyDependency
			}
		}
	}
	return errs
}

func downloadGz(r *requirement, validate bool, _ *models.NodeConfiguration, logger *slog.Logger) PreflightError {
	var errs PreflightError
	respGZ, err := http.Get(r.dlUrl)
	if err != nil {
		logger.Debug("failed to download artifact gzip", slog.String("artifact", r.name), slog.Any("err", err))
		return ErrFailedToDownload
	}
	defer respGZ.Body.Close()
	if respGZ.StatusCode != 200 {
		logger.Debug("failed to download rootfs gzip", slog.Any("status", respGZ.StatusCode))
		return ErrFailedToDownload
	}

	var hashBuffer bytes.Buffer
	gzipReader := io.TeeReader(respGZ.Body, &hashBuffer)
	uncompressedFile, err := gzip.NewReader(gzipReader)
	if err != nil {
		logger.Debug("failed to decompress gzip", slog.String("artifact", r.name), slog.Any("err", err))
		return ErrFailedToSatifyDependency
	}

	if validate {
		kgSha, err := getKnownGoodSha256FromURL(r.shaUrl)
		if err != nil {
			logger.Debug("failed to download known good shasum", slog.Any("err", err))
			err := checkContinue("Failed to download known good shasum for comparasion; continue?")
			if errors.Is(err, ErrUserCanceledPreflight) {
				return errs
			}
		}
		sha, err := validateSha256(&hashBuffer, kgSha)
		if err != nil {
			logger.Debug("sha256 mismatch", slog.String("expected", vmLinuxKernelSHA256), slog.String("actual", sha))
			err := checkContinue("Failed to download known good shasum for comparasion; continue?")
			if errors.Is(err, ErrUserCanceledPreflight) {
				return errors.Join(errs, ErrFailedToSatifyDependency)
			}
		}
	}

	outFile, err := os.Create(filepath.Join(r.path[0], r.name))
	if err != nil {
		logger.Debug("failed to create uncompressed file", slog.String("artifact", r.name), slog.Any("err", err))
		return ErrFailedToSatifyDependency
	}
	_, err = io.Copy(outFile, uncompressedFile)
	if err != nil {
		logger.Debug("failed to copy uncompressed file", slog.String("artifact", r.name), slog.Any("err", err))
		return ErrFailedToSatifyDependency
	}
	outFile.Close()

	return errs
}

func writeCniConf(r *requirement, _ bool, c *models.NodeConfiguration, logger *slog.Logger) PreflightError {
	f, err := os.Create(filepath.Join(r.path[0], r.name))
	if err != nil {
		logger.Debug("failed to create cni config file", slog.String("artifact", r.name), slog.Any("err", err))
		return ErrFailedToSatifyDependency
	}
	defer f.Close()

	tmpl, err := template.New("fcnet_conf").
		Funcs(template.FuncMap{
			"AddQuotes": addQuotes,
		}).
		Parse(templates.FcnetConfig)
	if err != nil {
		logger.Debug("failed to parse fcnet config template", slog.String("artifact", r.name), slog.Any("err", err))
		return ErrFailedToSatifyDependency
	}
	var buffer bytes.Buffer
	err = tmpl.Execute(&buffer, c.CNI)
	if err != nil {
		logger.Debug("failed to execute fcnet config template", slog.String("artifact", r.name), slog.Any("err", err))
		return ErrFailedToSatifyDependency
	}

	_, err = f.Write(buffer.Bytes())
	if err != nil {
		logger.Debug("failed to write fcnet config to file", slog.String("artifact", r.name), slog.Any("err", err))
		return ErrFailedToSatifyDependency
	}
	return nil
}
