package nexagent

import (
	"fmt"
	"os"
	"syscall"
)

const (
	VERSION = "0.0.1"
)

func HaltVM(err error) {
	// On the off chance the agent's log is captured from the vm
	fmt.Fprintf(os.Stderr, "Terminating Firecracker VM due to fatal error: %s\n", err)
	err = syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reboot: %s", err)
	}
}
