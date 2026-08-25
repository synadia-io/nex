package nex_test

// External test package: needs _test.StartNexus, which imports the root nex
// package (same rationale as node_undeploy_state_test.go).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nuid"

	"github.com/synadia-io/nex/_test"
	"github.com/synadia-io/nex/models"
)

// The state record key is "<register_type>_<workload_id>". GetStateByAgent
// matches records by the "<register_type>_" prefix, so a register type that
// itself contains "_" makes keys ambiguous: type "inmem" scanning prefix
// "inmem_" also claims type "inmem_x"'s key "inmem_x_wl1" and reconstructs
// the same colliding key on the re-read -- resume then starts another agent
// type's workload. The only complete guard is refusing "_" in the register
// type at registration.
func TestRegisterAgentRejectsUnderscoreType(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := _test.StartNatsServer(t, t.TempDir())
	defer s.Shutdown()

	nodes := _test.StartNexus(t, ctx, s.ClientURL(), 1, false)
	defer func() { _ = nodes[0].Shutdown() }()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	nodeID := _test.Node1Pub

	regB, err := json.Marshal(models.RegisterAgentRequest{
		Name:               "underscore-agent",
		RegisterType:       "inmem_x",
		StartRequestSchema: "{}",
	})
	be.NilErr(t, err)

	resp, err := nc.Request(models.AgentAPIRegisterRequestSubject(nuid.Next(), nodeID), regB, 5*time.Second)
	be.NilErr(t, err)
	if resp.Header.Get("Nats-Service-Error") == "" {
		t.Fatalf("registration with register_type %q was accepted; want rejection, got: %s", "inmem_x", string(resp.Data))
	}
	// Pin the SPECIFIC rejection, so this cannot pass vacuously off some
	// earlier check if one is ever added before it.
	if got := resp.Header.Get("Nats-Service-Error"); !strings.Contains(got, "register_type must not contain") {
		t.Fatalf("rejected for the wrong reason: %q", got)
	}
}
