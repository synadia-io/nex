package nex

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"disorder.dev/shandler"
	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-labs/nex/internal/state"
	"github.com/synadia-labs/nex/models"
)

func TestNexNodeOptions(t *testing.T) {
	t.Run("WithNodeName", func(t *testing.T) {
		t.Parallel()
		nn, err := NewNexNode(
			WithNodeName("nodetest"),
		)
		be.NilErr(t, err)
		be.Equal(t, "nodetest", nn.name)
	})
	t.Run("WithNexus", func(t *testing.T) {
		t.Parallel()
		nn, err := NewNexNode(
			WithNexus("nexustest"),
		)
		be.NilErr(t, err)
		be.Equal(t, "nexustest", nn.nexus)
	})
	t.Run("WithContext", func(t *testing.T) {
		t.Parallel()
		nn, err := NewNexNode(
			WithContext(context.WithValue(context.Background(), "nex", "test")), //nolint
		)
		be.NilErr(t, err)
		be.Equal(t, "test", nn.ctx.Value("nex"))
	})
	t.Run("WithLogger", func(t *testing.T) {
		t.Parallel()
		buf := new(bytes.Buffer)
		logger := slog.New(shandler.NewHandler(shandler.WithStdOut(buf)).WithGroup("nextest"))
		nn, err := NewNexNode(
			WithLogger(logger),
		)
		be.NilErr(t, err)
		be.DeepEqual(t, logger, nn.logger)

		nn.logger.Info("test")
		be.True(t, strings.Contains(buf.String(), "nextest"))
	})
	t.Run("WithNatsConn", func(t *testing.T) {
		t.Parallel()
		s := startNatsServer(t)
		defer s.Shutdown()
		nc, err := nats.Connect(s.ClientURL())
		be.NilErr(t, err)
		nn, err := NewNexNode(
			WithNatsConn(nc),
		)
		be.NilErr(t, err)
		be.DeepEqual(t, nc, nn.nc)
	})
	t.Run("WithInternalNatsServer", func(t *testing.T) {
		t.Parallel()
		opts := &server.Options{
			Port: -1,
		}
		nn, err := NewNexNode(
			WithInternalNatsServer(opts, &models.NatsConnectionData{}),
		)
		be.NilErr(t, err)

		nn.server.Start()
		defer nn.server.Shutdown()

		be.True(t, nn.server.ReadyForConnections(time.Second))
	})
	t.Run("WithNodeKeyPair", func(t *testing.T) {
		t.Parallel()
		kp, err := nkeys.CreateServer()
		be.NilErr(t, err)
		nn, err := NewNexNode(
			WithNodeKeyPair(kp),
		)
		be.NilErr(t, err)
		be.DeepEqual(t, kp, nn.nodeKeypair)
	})
	t.Run("WithNodeXKeyPair", func(t *testing.T) {
		t.Parallel()
		kp, err := nkeys.CreateCurveKeys()
		be.NilErr(t, err)
		nn, err := NewNexNode(
			WithNodeXKeyPair(kp),
		)
		be.NilErr(t, err)
		be.DeepEqual(t, kp, nn.nodeXKeypair)
	})
	t.Run("WithTag", func(t *testing.T) {
		t.Parallel()
		nn, err := NewNexNode(
			WithTag("nex", "test"),
		)
		be.NilErr(t, err)
		be.Equal(t, "test", nn.tags["nex"])
	})
	t.Run("WithAllowAgentRegistration", func(t *testing.T) {
		t.Parallel()
		nn, err := NewNexNode(
			WithAllowAgentRegistration(),
		)
		be.NilErr(t, err)
		be.True(t, nn.allowAgentRegistration)
	})
	t.Run("WithState", func(t *testing.T) {
		t.Parallel()
		nn, err := NewNexNode(
			WithState(&state.NoState{}),
		)
		be.NilErr(t, err)
		be.Nonzero(t, nn.state)
	})
	t.Run("WithSigningKey", func(t *testing.T) {
		kp, err := nkeys.CreateAccount()
		be.NilErr(t, err)
		pubKey, err := kp.PublicKey()
		be.NilErr(t, err)
		seed, err := kp.Seed()
		be.NilErr(t, err)

		nn, err := NewNexNode(
			WithSigningKey(string(seed), pubKey),
		)
		be.NilErr(t, err)
		be.Equal(t, string(seed), nn.signingKey)
		be.Equal(t, pubKey, nn.issuerAcct)
	})
	t.Run("WithMinter", func(t *testing.T) {
		t.Parallel()
		m := &minter{}
		nn, err := NewNexNode(
			WithMinter(m),
		)
		be.NilErr(t, err)
		mm, ok := nn.minter.(*minter)
		be.True(t, ok)
		be.DeepEqual(t, m, mm)
	})
	t.Run("WithAuctioneer", func(t *testing.T) {
		t.Parallel()
		a := &auction{}
		nn, err := NewNexNode(
			WithAuctioneer(a),
		)
		be.NilErr(t, err)
		aa, ok := nn.auctioneer.(*auction)
		be.True(t, ok)
		be.DeepEqual(t, a, aa)
	})
}

type auction struct{}

func (a *auction) Auction(namespace, _type string, aTags map[string]string) error {
	return nil
}

type minter struct{}

func (m *minter) MintRegister(agentId, nodeId string) (*models.NatsConnectionData, error) {
	return nil, nil
}

func (minter) Mint(typ models.CredType, namespace, id string) (*models.NatsConnectionData, error) {
	return nil, nil
}
