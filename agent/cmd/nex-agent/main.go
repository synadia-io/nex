//go:build linux

package main

import (
	"context"
	"fmt"
	"os"

	nexagent "github.com/synadia-io/nex/agent"
)

func main() {
	ctx, cancelF := context.WithCancel(context.Background())
	agent, err := nexagent.NewAgent(ctx, cancelF)
	if err != nil {
		nexagent.HaltVM(err)
		return
	}

	fmt.Fprintf(os.Stdout, "Starting Nex Agent - %s", agent.FullVersion())
	go agent.Start()

	<-ctx.Done()
}
