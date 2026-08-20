package nex_test

// External test package (same import-cycle reason as the other node_*_test.go
// files): this needs the root nex package to build real nodes.
//
// This is the multi-node regression for the owner-only STOP fix. In a nexus
// with more than one node, every node used to answer UNDEPLOY -- the nodes
// that do NOT hold the workload answering fast with "not found". Those fast
// negatives tripped the client's reply wait before the owning node's slower
// stop confirmation arrived, so a real, successful stop was reported to the
// operator as "workload not found". Non-owning nodes now stay silent (the
// same ownership-fetch / silent-drop the other verbs use), so only the owner
// answers and its slower confirmation is never raced.

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

	nex "github.com/synadia-io/nex"
	"github.com/synadia-io/nex/_test"
	tminter "github.com/synadia-io/nex/_test/minter"
	inmem "github.com/synadia-io/nex/_test/nexlet_inmem"
	nexclient "github.com/synadia-io/nex/client"
	iState "github.com/synadia-io/nex/internal/state"
	"github.com/synadia-io/nex/models"
)

// startNodeWithInmem brings up one node wired to its OWN in-memory nexlet and
// returns a handle to that nexlet, so a test can reach into exactly the node
// that ends up owning a workload (StartNexus threads shared runners to every
// node, which cannot give per-node handles).
func startNodeWithInmem(t *testing.T, ctx context.Context, url string, kp nkeys.KeyPair) *inmem.InMemAgent {
	t.Helper()

	pub, err := kp.PublicKey()
	be.NilErr(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	runner, agent, err := inmem.NewInMemAgentWithHandle("testnexus", pub, logger)
	be.NilErr(t, err)

	nc, err := nats.Connect(url)
	be.NilErr(t, err)
	t.Cleanup(nc.Close)

	sKV, err := iState.NewNatsKVState(nc, fmt.Sprintf("nex-%s", pub), logger)
	be.NilErr(t, err)

	node, err := nex.NewNexNode(
		nex.WithNodeKeyPair(kp),
		nex.WithNatsConn(nc),
		nex.WithNexus("testnexus"),
		nex.WithMinter(&tminter.TestMinter{NatsServers: []string{nc.ConnectedUrl()}}),
		nex.WithLogger(logger),
		nex.WithState(sKV),
		nex.WithAgentRunner(runner),
	)
	be.NilErr(t, err)
	be.NilErr(t, node.Start())
	be.NilErr(t, node.IsReady(30*time.Second))
	t.Cleanup(func() { _ = node.Shutdown() })

	return agent
}

func TestMultiNodeStopConfirmedDespiteSlowOwner(t *testing.T) {
	workDir := t.TempDir()
	server := _test.StartNatsServer(t, workDir)
	defer server.Shutdown()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	kpA, err := nkeys.CreateServer()
	be.NilErr(t, err)
	kpB, err := nkeys.CreateServer()
	be.NilErr(t, err)

	// Two nodes, one nexus. Both run an inmem nexlet, so both bid on the
	// auction and both receive the UNDEPLOY broadcast; only one ends up
	// holding the workload.
	agentA := startNodeWithInmem(t, ctx, server.ClientURL(), kpA)
	agentB := startNodeWithInmem(t, ctx, server.ClientURL(), kpB)

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	client, err := nexclient.NewClient(context.Background(), nc, "user")
	be.NilErr(t, err)

	var ar []*models.AuctionResponse
	_test.WaitFor(t, 10*time.Second, func() bool {
		ar, err = client.Auction("user", "inmem", map[string]string{})
		return err == nil && len(ar) == 2
	}, "waiting for both nodes to bid on the auction")

	sr, err := client.StartWorkload(ar[0].BidderId, &models.StartWorkloadRequest{
		Namespace:         "user",
		Name:              "tester",
		RunRequest:        "{}",
		WorkloadType:      "inmem",
		WorkloadLifecycle: models.WorkloadLifecycleService,
	})
	be.NilErr(t, err)

	// Find the node that actually owns the workload and make ONLY its stop
	// slow -- slower than the client's inter-message stall (2s), which is
	// exactly the delay that let the other node's fast "not found" win
	// before the fix. The non-owner's stop is left instant; under owner-only
	// it never runs anyway (that node silent-drops after its ownership
	// fetch), but leaving it instant is what makes this discriminating: a
	// regression re-introduces the fast negative.
	_test.WaitFor(t, 5*time.Second, func() bool {
		return agentA.PingWorkload(sr.Id) || agentB.PingWorkload(sr.Id)
	}, "waiting for the workload to be held by one of the nodes")

	if agentA.PingWorkload(sr.Id) {
		agentA.StopDelay = 3 * time.Second
	} else {
		agentB.StopDelay = 3 * time.Second
	}

	resp, err := client.StopWorkload(sr.Id)
	be.NilErr(t, err)

	// The decisive assertion: the slow-but-successful stop is reported as
	// stopped, not misreported as not-found by a faster non-owner.
	be.Equal(t, sr.Id, resp.Id)
	be.True(t, resp.Stopped)

	// And it is actually gone from whichever node held it.
	be.False(t, agentA.PingWorkload(sr.Id))
	be.False(t, agentB.PingWorkload(sr.Id))
}
