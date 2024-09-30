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
)

type Upgrade struct {
	GitTag string `json:"tag_name" help:"Specific release tag to download"`

	installVersion string `kong:"-"`
}

func versionCheck() (string, error) {
	if COMMIT == "development" {
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
🎉 There is a newer version [v%s] of the Nex available 🎉
To update, run:
     nex upgrade
================================================================

`,
			latestTag)
	}

	return latestTag, nil
}

func (u Upgrade) Run(ctx context.Context, globals *Globals) error {
	if globals.Check {
		return printTable("Node Upgrade Configuration", append(globals.Table(), u.Table()...)...)
	}

	var err error
	if u.GitTag == "" {
		u.installVersion, err = versionCheck()
	} else {
		u.installVersion = u.GitTag
	}

	if u.installVersion == VERSION {
		// no upgrade needed
		return nil
	}

	if u.installVersion == "" {
		return errors.New("upgrade disabled when using a developmental build of nex cli")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	nexPath, err := os.Executable()
	if err != nil {
		return err
	}

	f, err := os.Open(nexPath)
	if err != nil {
		return err
	}

	// copy binary backup
	f_bak, err := os.Create(nexPath + ".bak")
	if err != nil {
		return err
	}
	defer os.Remove(nexPath + ".bak")
	_, err = io.Copy(f_bak, f)
	if err != nil {
		return err
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

	url := getUpgradeURL(u.installVersion)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download nex: %s | %s", resp.Status, url)
	}

	nexBinary, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	nex, err := os.Create(nexPath + ".new")
	if err != nil {
		return err
	}
	defer os.Remove(nexPath + ".new")

	_, err = nex.Write(nexBinary)
	if err != nil {
		return err
	}

	h := sha256.New()
	if _, err := io.Copy(h, nex); err != nil {
		return err
	}

	err = nex.Close()
	if err != nil {
		return err
	}

	shasum := hex.EncodeToString(h.Sum(nil))

	err = os.Rename(nexPath+".new", nexPath)
	if err != nil {
		restoreBackup()
		return err
	}

	err = os.Chmod(nexPath, 0755)
	if err != nil {
		logger.Error("Failed to update nex binary permissions", slog.Any("err", err))
	}

	logger.Debug("New binary downloaded", slog.String("sha256", shasum))
	logger.Info("nex upgrade complete!", slog.String("new_version", u.installVersion))

	return nil
}
