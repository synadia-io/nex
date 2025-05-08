package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"

	"github.com/nats-io/nkeys"
	"github.com/synadia-labs/nex/models"

	inmem "github.com/synadia-labs/nex/_test/nexlet_inmem"
)

var (
	VERSION   string = "0.0.0"
	COMMIT    string = ""
	BUILDDATE string = ""
)

var exitCode int = 1

func main() {
	defer os.Exit(exitCode) //nolint

	actx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		cancel()
	}()

	showTestLogsEnv, err := strconv.ParseBool(os.Getenv("NEX_TEST_LOGS"))
	if err != nil {
		showTestLogsEnv = false
	}
	logger := slog.New(slog.NewTextHandler(func() io.Writer {
		if showTestLogsEnv {
			return os.Stdout
		}
		return io.Discard
	}(), &slog.HandlerOptions{Level: slog.LevelDebug}))

	agentId, ok := os.LookupEnv("NEX_AGENT_ASSIGNED_ID")
	if !ok {
		slog.Error("NEX_AGENT_ASSIGNED_ID is required")
		return
	}

	natsUrl, ok := os.LookupEnv("NEX_AGENT_NATS_URL")
	if !ok {
		slog.Error("NEX_AGENT_NATS_URL is required")
		return
	}

	nodeId, ok := os.LookupEnv("NEX_AGENT_NODE_ID")
	if !ok {
		slog.Error("NEX_AGENT_NODE_ID is required")
		return
	}
	if !nkeys.IsValidPublicServerKey(nodeId) {
		slog.Error("NEX_AGENT_NODE_ID is not a valid node ID")
		return
	}

	myAgent, err := inmem.NewInMemAgent(nodeId, logger)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	// launch the agent
	if err := myAgent.Run(agentId, models.NatsConnectionData{NatsUrl: natsUrl}); err != nil {
		slog.Error(err.Error())
		return
	}

	<-actx.Done()
	exitCode = 0
}
