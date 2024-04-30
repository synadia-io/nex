package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime"
)

func versionCheck() (string, error) {
	if VERSION == "development" {
		return "", nil
	}

	res, err := http.Get("https://api.github.com/repos/synadia-io/nex/releases/latest")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	payload := make(map[string]interface{})
	err = json.Unmarshal(b, &payload)
	if err != nil {
		return "", err
	}

	latestTag, ok := payload["tag_name"].(string)
	if !ok {
		return "", errors.New("error parsing tag_name")
	}

	if latestTag != VERSION {
		fmt.Printf(`================================================================
🎉 There is a newer version [v%s] of the NEX CLI available 🎉
To update, run:
     nex upgrade
================================================================

`,
			latestTag)
	}

	return latestTag, nil
}

func UpgradeNex(ctx context.Context, logger *slog.Logger, newVersion string) (string, error) {
	nexPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	f, err := os.Open(nexPath)
	if err != nil {
		return "", err
	}

	// copy binary backup
	f_bak, err := os.Create(nexPath + ".bak")
	if err != nil {
		return "", err
	}
	defer os.Remove(nexPath + ".bak")
	_, err = io.Copy(f_bak, f)
	if err != nil {
		return "", err
	}
	f_bak.Close()
	f.Close()

	restoreBackup := func() {
		logger.Info("Restoring backup binary")
		if err := os.Rename(nexPath+".bak", nexPath); err != nil {
			logger.Error("Failed to restore backup binary", slog.Any("err", err))
		}
		err = os.Chmod(nexPath, 0755)
		if err != nil {
			logger.Error("Failed to restore backup binary permissions", slog.Any("err", err))
		}
	}

	_os := runtime.GOOS
	arch := runtime.GOARCH

	url := fmt.Sprintf("https://github.com/synadia-io/nex/releases/download/%s/nex_%s_%s_%s", newVersion, newVersion, _os, arch)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download nex: %s", resp.Status)
	}

	nexBinary, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	nex, err := os.Create(nexPath + ".new")
	if err != nil {
		return "", err
	}
	defer os.Remove(nexPath + ".new")

	_, err = nex.Write(nexBinary)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	if _, err := io.Copy(h, nex); err != nil {
		return "", err
	}

	shasum := hex.EncodeToString(h.Sum(nil))

	err = os.Rename(nexPath+".new", nexPath)
	if err != nil {
		restoreBackup()
		return "", err
	}

	err = os.Chmod(nexPath, 0755)
	if err != nil {
		logger.Error("Failed to restore backup binary permissions", slog.Any("err", err))
	}

	logger.Debug("New binary downloaded", slog.String("sha256", shasum))
	logger.Info("nex upgrade complete!", slog.String("new_version", newVersion))

	return shasum, nil
}
