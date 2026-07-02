package nex

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	tminter "github.com/synadia-io/nex/_test/minter"
	inmem "github.com/synadia-io/nex/_test/nexlet_inmem"
	"github.com/synadia-io/nex/models"
)

// flakyMinter fails the first failures calls of each method before delegating
// to the wrapped minter, simulating a transient control-plane outage.
type flakyMinter struct {
	inner    models.CredVendor
	failures int

	mu            sync.Mutex
	registerCalls int
	mintCalls     int
}

func (m *flakyMinter) MintRegister(agentId, nodeId string) (*models.NatsConnectionData, error) {
	m.mu.Lock()
	m.registerCalls++
	fail := m.registerCalls <= m.failures
	m.mu.Unlock()
	if fail {
		return nil, errors.New("transient minter failure")
	}
	return m.inner.MintRegister(agentId, nodeId)
}

func (m *flakyMinter) Mint(typ models.CredType, namespace, id string) (*models.NatsConnectionData, error) {
	m.mu.Lock()
	m.mintCalls++
	fail := m.mintCalls <= m.failures
	m.mu.Unlock()
	if fail {
		return nil, errors.New("transient minter failure")
	}
	return m.inner.Mint(typ, namespace, id)
}

// TestNodeStartSurvivesTransientMintFailures proves that a minter which fails
// its first attempts (e.g. a control-plane-backed vendor hiccuping) no longer
// permanently skips agent startup: both the node-side MintRegister and the
// registration handler's Mint are retried with backoff.
func TestNodeStartSurvivesTransientMintFailures(t *testing.T) {
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

	r, err := inmem.NewInMemAgent("nexus", pub, logger)
	be.NilErr(t, err)

	minter := &flakyMinter{
		inner:    &tminter.TestMinter{NatsServers: []string{s.ClientURL()}},
		failures: 2,
	}

	nn, err := NewNexNode(
		WithNatsConn(nc),
		WithLogger(logger),
		WithNodeKeyPair(kp),
		WithAgentRunner(r),
		WithMinter(minter),
	)
	be.NilErr(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, nn.Shutdown())
	}()

	be.NilErr(t, nn.Start())
	be.NilErr(t, nn.IsReady(10*time.Second))

	be.Equal(t, 1, nn.registeredAgents.Count())

	minter.mu.Lock()
	registerCalls, mintCalls := minter.registerCalls, minter.mintCalls
	minter.mu.Unlock()
	// Both methods must have been retried past their injected failures.
	be.True(t, registerCalls > minter.failures)
	be.True(t, mintCalls > minter.failures)

	cancel()
	be.NilErr(t, nn.WaitForShutdown())
}
