//go:build linux || darwin

package main

import (
	"fmt"
	"runtime"
)

func getUpgradeURL(version string) string {
	_os := runtime.GOOS
	arch := runtime.GOARCH

	return fmt.Sprintf("https://github.com/synadia-io/nex/releases/download/%s/nex_%s_%s_%s", version, version, _os, arch)
}
