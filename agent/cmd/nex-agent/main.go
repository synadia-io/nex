//go:build linux

package main

import (
	"context"
	"fmt"

	nexagent "github.com/synadia-io/nex/agent"
)

func main() {
	ctx, cancelF := context.WithCancel(context.Background())
	agent, err := nexagent.NewAgent(ctx, cancelF)
	if err != nil {
		fmt.Printf("Failed to start agent - %s\n", err)
		nexagent.HaltVM(err)
		return
	}

	fmt.Printf("Starting Nex Agent - %s\n", agent.FullVersion())
	go agent.Start()

	<-ctx.Done()
}
