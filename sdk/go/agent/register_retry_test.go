package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/models"
)

// TestRemoteAgentInitRetriesUntilNodeResponds proves the registration request
// is retried with backoff: the node's responder only appears after the first
// attempt would already have failed with ErrNoResponders. Before the retry
// change, RemoteAgentInit failed instantly in this scenario.
func TestRemoteAgentInitRetriesUntilNodeResponds(t *testing.T) {
	s := server.New(&server.Options{Port: -1})
	s.Start()
	defer s.Shutdown()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats server failed to start")
	}

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	const nexus = "retry-test-nexus"

	// First attempt fails fast (no responders). Bring the responder up while
	// the retry loop is backing off.
	go func() {
		time.Sleep(400 * time.Millisecond)

		snc, err := nats.Connect(s.ClientURL())
		if err != nil {
			return
		}

		_, err = snc.Subscribe(models.AgentAPIInitRemoteRegisterRequestSubject(nexus), func(m *nats.Msg) {
			resp, _ := json.Marshal(models.RegisterRemoteAgentResponse{
				AssignedAgentId: "agent-123",
				RespondTo:       "node-abc",
			})
			_ = m.Respond(resp)
		})
		if err != nil {
			return
		}
		_ = snc.Flush()
	}()

	resp, err := RemoteAgentInit(nc, nexus, "unused")
	be.NilErr(t, err)
	be.Equal(t, "agent-123", resp.AssignedAgentId)
}
