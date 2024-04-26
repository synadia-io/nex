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
	"os/exec"
	"path/filepath"
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
     nex update
================================================================

`,
			latestTag)
	}

	return latestTag, nil
}

func UpgradeNex(ctx context.Context, logger *slog.Logger, newVersion string) (string, error) {
	nexPath, err := exec.LookPath("nex")
	if err != nil {
		return "", err
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

	dir, err := os.MkdirTemp(os.TempDir(), "nex-upgrade-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	nexBinary, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	nex, err := os.Create(filepath.Join(dir, "nex"))
	if err != nil {
		return "", err
	}

	_, err = nex.Write(nexBinary)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	if _, err := io.Copy(h, nex); err != nil {
		return "", err
	}

	shasum := hex.EncodeToString(h.Sum(nil))
	logger.Debug("New binary downloaded", slog.String("sha256", shasum))

	err = os.Chmod(filepath.Join(dir, "nex"), 0775)
	if err != nil {
		return "", err
	}

	err = os.Rename(filepath.Join(dir, "nex"), nexPath)
	if err != nil {
		return "", err
	}

	logger.Info("nex upgrade complete!", slog.String("new_version", newVersion))

	return shasum, nil
}
