//go:build !linux

package nexagent

import (
	"fmt"
	"os"
)

func HaltVM(err error) {
	code := 0
	if err != nil {
		fmt.Fprintf(os.Stderr, "Terminating process due to fatal error: %s. Sandboxed: %v\n", err, isSandboxed())
		code = 1
	}

	os.Exit(code)
}
