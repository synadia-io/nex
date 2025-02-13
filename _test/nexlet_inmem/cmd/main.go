package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/nats-io/nkeys"
	"github.com/synadia-labs/nex/models"

	inmem "github.com/synadia-labs/nex/_test/nexlet_inmem"
)

var (
	VERSION   string = "0.0.0"
	COMMIT    string = ""
	BUILDDATE string = ""
)

var exitCode int = 0

func main() {
	defer os.Exit(exitCode) //nolint

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		cancel()
	}()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	agentId, ok := os.LookupEnv("NEX_AGENT_ASSIGNED_ID")
	if !ok {
		slog.Error("NEX_AGENT_ASSIGNED_ID is required")
		exitCode = 1
		return
	}

	natsUrl, ok := os.LookupEnv("NEX_AGENT_NATS_URL")
	if !ok {
		slog.Error("NEX_AGENT_NATS_URL is required")
		exitCode = 1
		return
	}

	nodeId, ok := os.LookupEnv("NEX_AGENT_NODE_ID")
	if !ok {
		slog.Error("NEX_AGENT_NODE_ID is required")
		exitCode = 1
		return
	}
	if nkeys.IsValidPublicServerKey(nodeId) {
		slog.Error("NEX_AGENT_NODE_ID is not a valid node ID")
		exitCode = 1
		return
	}

	myAgent, err := inmem.NewInMemAgent(nodeId, logger)
	if err != nil {
		slog.Error(err.Error())
		exitCode = 1
		return
	}

	// launch the agent
	if err := myAgent.Run(ctx, agentId, models.NatsConnectionData{NatsUrl: natsUrl}); err != nil {
		slog.Error(err.Error())
		exitCode = 1
		return
	}
}
