package internal

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

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

// TestAgentRegistrationsOfflineIsNotEvictedImmediately pins the two-stage
// health policy: a missed-heartbeat agent is marked Offline (and so excluded
// from placement, which filters on Healthy) but stays registered, because its
// heartbeat subscription is still live and the next heartbeat restores it --
// agents register exactly once at startup, so an eviction here is permanent
// for a live-but-briefly-silent agent (a >30s NATS blip used to orphan every
// agent on the node at once). Only an agent silent past the hard TTL is
// evicted to keep the map bounded.
func TestAgentRegistrationsOfflineIsNotEvictedImmediately(t *testing.T) {
	s := startNatsServer(t, t.TempDir())
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ar := NewAgentRegistrations(t.Context(), nodePub, nc, logger)

	reg := &AgentRegistration{ID: "agent-blip", RegisterRequest: &models.RegisterAgentRequest{RegisterType: "inmem"}}
	be.NilErr(t, ar.Add(reg))

	// Silent past the offline threshold, but nowhere near the hard TTL.
	reg.rwLock.Lock()
	reg.lastHeartbeat = time.Now().Add(-time.Minute)
	reg.rwLock.Unlock()

	ar.sweep()

	be.Equal(t, 1, ar.Count())
	reg.rwLock.RLock()
	be.Equal(t, AgentOffline, reg.HealthStatus)
	reg.rwLock.RUnlock()

	// The heartbeat subscription survived the Offline grading, so the next
	// heartbeat restores the agent -- the recovery an eviction forecloses.
	hbB, err := json.Marshal(models.AgentHeartbeat{Summary: models.AgentSummary{State: string(models.AgentStateRunning)}})
	be.NilErr(t, err)

	// Published inside the poll: the heartbeat subscription is set up
	// asynchronously by Add, so a single publish could race it and vanish.
	deadline := time.Now().Add(5 * time.Second)
	for {
		be.NilErr(t, nc.Publish(models.AgentAPIHeartbeatSubject(nodePub, "agent-blip"), hbB))
		be.NilErr(t, nc.Flush())

		reg.rwLock.RLock()
		status := reg.HealthStatus
		reg.rwLock.RUnlock()
		if status == AgentHealthy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("agent did not recover to Healthy after heartbeat; status %v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestAgentRegistrationsHardTTLEvicts pins the bounded-map half: an agent
// silent past the hard TTL is genuinely gone (a crashed agent's replacement
// registers under a NEW id, so nothing will ever heartbeat this one again)
// and is evicted.
func TestAgentRegistrationsHardTTLEvicts(t *testing.T) {
	s := startNatsServer(t, t.TempDir())
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ar := NewAgentRegistrations(t.Context(), nodePub, nc, logger)

	reg := &AgentRegistration{ID: "agent-dead", RegisterRequest: &models.RegisterAgentRequest{RegisterType: "inmem"}}
	be.NilErr(t, ar.Add(reg))

	reg.rwLock.Lock()
	reg.lastHeartbeat = time.Now().Add(-agentHardEvictAfter - time.Second)
	reg.rwLock.Unlock()

	ar.sweep()

	be.Equal(t, 0, ar.Count())
}
