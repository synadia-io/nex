//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootFsSimple(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping test; requires root privileges")
	}

	rfs := RootFS{
		OciRef:    "docker.io/synadia/nex-rootfs:alpine",
		Os:        "linux",
		Arch:      "amd64",
		OutputDir: t.TempDir(),
	}

	err := rfs.Run()
	if err != nil {
		t.Fatalf("Error running rootfs: %v", err)
	}

	output, err := exec.Command("file", filepath.Join(rfs.OutputDir, "rootfs.ext4")).CombinedOutput()
	if err != nil {
		t.Fatalf("Error running file: %v", err)
	}

	if !strings.Contains(string(output), "Linux rev 1.0 ext4 filesystem data") {
		t.Fatalf("did not build ext4 filesystem")
	}
}
