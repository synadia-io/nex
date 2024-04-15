package nexagent

import (
	"fmt"
	"os"
	"syscall"
)

func HaltVM(err error) {
	code := 0
	if err != nil {
		fmt.Fprintf(os.Stderr, "Terminating process due to fatal error: %s. Sandboxed: %v\n", err, isSandboxed())
		code = 1
	}

	if isSandboxed() {
		err = syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to halt: %s", err)
		}
	} else {
		os.Exit(code)
	}
}
