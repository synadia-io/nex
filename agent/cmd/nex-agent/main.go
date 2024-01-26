//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"syscall"

	nexagent "github.com/synadia-io/nex/agent"
)

func main() {
	ctx := context.Background()

	agent, err := nexagent.NewAgent()
	if err != nil {
		haltVM(err)
	}

	fmt.Fprintf(os.Stdout, "Starting NEX Agent - %s", agent.FullVersion())
	if agent != nil {
		err = agent.Start()
		if err != nil {
			haltVM(err)
		}
	}

	<-ctx.Done()
}

// haltVM stops the firecracker VM
func haltVM(err error) {
	// On the off chance the agent's log is captured from the vm
	fmt.Fprintf(os.Stderr, "Terminating Firecracker VM due to fatal error: %s\n", err)
	err = syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reboot: %s", err)
	}
}
