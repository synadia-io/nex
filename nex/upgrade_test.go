package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"disorder.dev/shandler"
)

func logger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(shandler.NewHandler())
}

func setEnvironment(t *testing.T) {
	t.Helper()

	tempPath := t.TempDir()
	f, err := os.Create(filepath.Join(tempPath, "nex"))
	if err != nil {
		t.Fail()
	}
	f.Close()

	err = os.Chmod(filepath.Join(tempPath, "nex"), 0775)
	if err != nil {
		t.Fail()
	}

	path := os.Getenv("PATH")
	os.Setenv("PATH", tempPath+":"+path)
}

func TestUpdateNex(t *testing.T) {
	setEnvironment(t)
	log := logger(t)

	testNexPath, _ := exec.LookPath("nex")
	t.Log("nex path: " + testNexPath)
	if !strings.HasPrefix(testNexPath, os.TempDir()) {
		t.Log("bailing on update nex test so real env isnt affected")
		t.SkipNow()
	}

	shasum, err := UpgradeNex(context.Background(), log, "0.2.1")
	if err != nil {
		t.Fatal(err)
	}

	t.Log(shasum)
}
