package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/nats-io/nkeys"
	agent "github.com/synadia-io/nexlet.go/agent"
	inmem "github.com/synadia-io/nexlet.go/examples/memory"
	"github.com/synadia-labs/nex/models"
)

const (
	agentName string = "inmem"
)

var (
	VERSION   string = "0.0.0"
	COMMIT    string = ""
	BUILDDATE string = ""
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		cancel()
	}()

	xkp, err := nkeys.CreateCurveKeys()
	if err != nil {
		panic(fmt.Errorf("failed to create key pair: %w", err))
	}

	myAgent := &inmem.InMemAgent{
		Name:      agentName,
		Version:   VERSION,
		XPair:     xkp,
		StartTime: time.Now(),
	}

	opts := []agent.RunnerOpt{}
	nodeId, ok := os.LookupEnv("NEX_AGENT_NODE_ID")
	if !ok {
		slog.Error("NEX_AGENT_NODE_ID is required")
		return
	}
	if !nkeys.IsValidPublicServerKey(nodeId) {
		slog.Error("NEX_AGENT_NODE_ID is not a valid public server key")
		return
	}

	agentId, ok := os.LookupEnv("NEX_AGENT_ASSIGNED_ID")
	if !ok {
		slog.Error("NEX_AGENT_ASSIGNED_ID is required")
		return
	}

	runner, err := agent.NewRunner(ctx, nodeId, myAgent, opts...)
	if err != nil {
		panic(fmt.Errorf("failed to create runner: %w", err))
	}

	// launch the agent
	if err := runner.Run(agentId, models.NatsConnectionData{}); err != nil {
		panic(fmt.Errorf("failed to run agent: %w", err))
	}
}
