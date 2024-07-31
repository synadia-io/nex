package nexagent

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func HaltVM(err error) {
	code := 0
	if err != nil {
		fmt.Fprintf(os.Stderr, "Terminating process due to fatal error: %s. Sandboxed: %v\n", err, isSandboxed())
		code = 1
	}

	// don't call reboot if we expected this halt (e.g. captured sigterm)
	if isSandboxed() && err != nil {
		err = syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to halt: %s", err)
		}
	} else {
		os.Exit(code)
	}
}

func resetSIGUSR() {
	signal.Reset(syscall.SIGUSR1, syscall.SIGUSR2)
}
