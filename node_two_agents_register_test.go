package nex

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	tminter "github.com/synadia-io/nex/_test/minter"
	inmem "github.com/synadia-io/nex/_test/nexlet_inmem"
)

func TestNodeRegistersEveryEmbeddedAgent(t *testing.T) {
	const (
		iterations     = 8
		agentsPerNode  = 2
		readyTimeout   = 10 * time.Second
		shutdownWindow = 30 * time.Second
	)

	for i := range iterations {
		t.Run(fmt.Sprintf("start_%d", i), func(t *testing.T) {
			s := startNatsServer(t)
			defer s.Shutdown()

			nc, err := nats.Connect(s.ClientURL())
			be.NilErr(t, err)
			defer nc.Close()

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))

			kp, err := nkeys.CreateServer()
			be.NilErr(t, err)

			pub, err := kp.PublicKey()
			be.NilErr(t, err)

			// Two agents with distinct names and register types, mirroring
			// nex-ce's connector/container pair. Distinct ids matter: Add is
			// keyed by agent id, so this is not a duplicate-key collision.
			connector, err := inmem.NewInMemAgent("nexus", pub, logger,
				inmem.WithAgentName("connector-inmem"),
				inmem.WithWorkloadType("connector"),
			)
			be.NilErr(t, err)

			container, err := inmem.NewInMemAgent("nexus", pub, logger,
				inmem.WithAgentName("container-inmem"),
				inmem.WithWorkloadType("container"),
			)
			be.NilErr(t, err)

			nn, err := NewNexNode(
				WithNatsConn(nc),
				WithLogger(logger),
				WithNodeKeyPair(kp),
				WithAgentRunner(connector),
				WithAgentRunner(container),
				WithMinter(&tminter.TestMinter{
					NatsServers: []string{s.ClientURL()},
				}),
			)
			be.NilErr(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), shutdownWindow)
			defer cancel()

			go func() {
				<-ctx.Done()
				_ = nn.Shutdown()
			}()

			be.NilErr(t, nn.Start())
			be.NilErr(t, nn.IsReady(readyTimeout))

			// Start() calls WaitForAgents() before returning, so by here every
			// agent that is ever going to register already has. A short count
			// is permanent, not pending.
			be.Equal(t, agentsPerNode, nn.registeredAgents.Count())

			// Both register types must be placeable. GetByRegisterType is what
			// handleAuction consults, and it only returns healthy agents, so a
			// lookup failure here is exactly the auction drop seen in nex-ce.
			for _, registerType := range []string{"connector", "container"} {
				reg, err := nn.registeredAgents.GetByRegisterType(registerType)
				be.NilErr(t, err)
				be.Nonzero(t, reg)
			}

			cancel()
			be.NilErr(t, nn.WaitForShutdown())
		})
	}
}
