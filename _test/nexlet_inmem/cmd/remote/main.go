package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nexlet.go/agent"

	inmem "github.com/synadia-labs/nex/_test/nexlet_inmem"
)

var (
	VERSION   string = "0.0.0"
	COMMIT    string = ""
	BUILDDATE string = ""
)

var (
	exitCode int = 1
	nexus        = flag.String("nexus", "nexus", "Nexus name to use for the agent")
)

func main() {
	defer os.Exit(exitCode) //nolint
	flag.Parse()

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

	actx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		cancel()
	}()

	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		slog.Error("Failed to connect to NATS", "error", err)
		return
	}

	xkp, err := nkeys.CreateCurveKeys()
	if err != nil {
		slog.Error("Failed to create NKEYS", "error", err)
		return
	}

	pubKey, err := xkp.PublicKey()
	if err != nil {
		slog.Error("Failed to get public key", "error", err)
		return
	}

	rrr, err := agent.RemoteAgentInit(nc, *nexus, pubKey)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	myAgent, err := inmem.NewInMemAgent(*nexus, rrr.RespondTo, logger)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	// launch the agent
	if err := myAgent.Run(rrr.AssignedAgentId, *rrr.RegistrationCreds); err != nil {
		slog.Error(err.Error())
		return
	}

	<-actx.Done()
	exitCode = 0
}
