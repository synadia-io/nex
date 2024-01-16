//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"syscall"

	nexagent "github.com/ConnectEverything/nex/agent"
)

var (
	VERSION   = "development"
	COMMIT    = ""
	BUILDDATE = ""
)

func main() {
	fmt.Fprintf(os.Stdout, "Starting NEX Agent - %s [%s] BuildDate: %s\n", VERSION, COMMIT, BUILDDATE)
	ctx := context.Background()

	agent, err := nexagent.NewAgent()
	if err != nil {
		haltVM(err)
	}

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
	fmt.Fprintf(os.Stderr, "Terminating Firecracker VM due to fatal error: %s\n%s [%s] BuildDate: %s\n", err, VERSION, COMMIT, BUILDDATE)
	err = syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reboot: %s", err)
	}
}
