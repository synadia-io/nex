package internal

import (
	"io"
	"log/slog"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"

	"github.com/synadia-io/nex/models"
)

// TestAgentRegistrationsRemoveFreesID pins the eviction half of the crash-
// recovery fix: the duplicate-ID guard blocks a second registration under a
// live id (correct), but once the holder is evicted the id is free again, so
// a fresh registration under it succeeds. Without Remove, a dead agent's id
// stayed taken forever.
func TestAgentRegistrationsRemoveFreesID(t *testing.T) {
	s := startNatsServer(t, t.TempDir())
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ar := NewAgentRegistrations(t.Context(), nodePub, nc, logger)

	reg := &AgentRegistration{ID: "agent-1", RegisterRequest: &models.RegisterAgentRequest{RegisterType: "inmem"}}
	be.NilErr(t, ar.Add(reg))
	be.Equal(t, 1, ar.Count())

	// The duplicate guard rejects a second registration while the id is held.
	dup := &AgentRegistration{ID: "agent-1", RegisterRequest: &models.RegisterAgentRequest{RegisterType: "inmem"}}
	be.Nonzero(t, ar.Add(dup))

	// After eviction the id is free and a fresh registration under it is
	// accepted -- the wall a crashed agent used to hit permanently.
	ar.Remove("agent-1")
	be.Equal(t, 0, ar.Count())

	be.NilErr(t, ar.Add(dup))
	be.Equal(t, 1, ar.Count())
}
