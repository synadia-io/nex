package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"time"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/sdk/go/utils"
	"github.com/synadia-io/nex/models"

	inmem "github.com/synadia-io/nex/_test/nexlet_inmem"
	eventemitter "github.com/synadia-io/nex/internal/event_emitter"
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

	nodeId, ok := os.LookupEnv("NEX_AGENT_NODE_ID")
	if !ok {
		slog.Error("NEX_AGENT_NODE_ID is required")
		return
	}
	if !nkeys.IsValidPublicServerKey(nodeId) {
		slog.Error("NEX_AGENT_NODE_ID is not a valid node ID")
		return
	}

	myAgent, err := inmem.NewInMemAgent(*nexus, nodeId, logger)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	natsConnData, err := utils.NatsConnDataFromEnv()
	if err != nil {
		slog.Error(err.Error())
		return
	}

	// create nats connection for emitter
	nc, err := configureNatsConnection(natsConnData)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	// launch the agent
	if err := myAgent.Run(agentId, *natsConnData, eventemitter.NewNatsEmitter(actx, nc)); err != nil {
		slog.Error(err.Error())
		return
	}

	<-actx.Done()
	exitCode = 0
}

func configureNatsConnection(connData *models.NatsConnectionData) (*nats.Conn, error) {
	if connData.ConnName == "" {
		connData.ConnName = "inmem_agent"
	}

	opts := []nats.Option{
		nats.Name(connData.ConnName),
		nats.MaxReconnects(-1),
		nats.Timeout(10 * time.Second),
	}

	if connData.TlsCert != "" && connData.TlsKey != "" {
		opts = append(opts, nats.ClientCert(connData.TlsCert, connData.TlsKey))
	}
	if connData.TlsCa != "" {
		opts = append(opts, nats.RootCAs(connData.TlsCa))
	}
	if connData.TlsFirst {
		opts = append(opts, nats.TLSHandshakeFirst())
	}

	switch {
	case connData.NatsUserSeed != "" && connData.NatsUserJwt != "": // Use seed + jwt
		opts = append(opts, nats.UserJWTAndSeed(connData.NatsUserJwt, connData.NatsUserSeed))
	case connData.NatsUserNkey != "" && connData.NatsUserSeed != "": // User nkey
		opts = append(opts, nats.Nkey(connData.NatsUserNkey, func(nonce []byte) ([]byte, error) {
			kp, err := nkeys.FromSeed([]byte(connData.NatsUserSeed))
			if err != nil {
				return nil, err
			}
			return kp.Sign(nonce)
		}))
	case connData.NatsUserName != "" && connData.NatsUserPassword != "": // Use user + password
		opts = append(opts, nats.UserInfo(connData.NatsUserName, connData.NatsUserPassword))
	}

	if len(connData.NatsServers) == 0 {
		connData.NatsServers = []string{nats.DefaultURL}
	}

	nc, err := nats.Connect(strings.Join(connData.NatsServers, ","), opts...)
	if err != nil {
		return nil, err
	}

	return nc, nil
}
