package nex_test

// This file is an EXTERNAL test package (nex_test), not the internal
// node_test.go (package nex). It has to be: the test below needs
// _test.StartNexus/StartNexusWithOptions (github.com/synadia-io/nex/_test),
// and that package imports the root nex package -- an internal test file
// (package nex) importing _test would be an import cycle ("import cycle not
// allowed in test"). Being external means this file cannot reach NexNode's
// unexported `state` field directly; the state decorator below is instead
// injected the sanctioned way, through nex.WithState (see options.go),
// threaded in via the new StartNexusWithOptions extraOpts parameter.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nuid"

	nex "github.com/synadia-io/nex"
	"github.com/synadia-io/nex/_test"
	inmem "github.com/synadia-io/nex/_test/nexlet_inmem"
	iState "github.com/synadia-io/nex/internal/state"
	"github.com/synadia-io/nex/models"
)

// removeWorkloadCall records one call to recordingState.RemoveWorkload.
type removeWorkloadCall struct {
	workloadType string
	workloadId   string
}

// storeWorkloadCall records one call to recordingState.StoreWorkload.
type storeWorkloadCall struct {
	workloadId string
	request    models.StartWorkloadRequest
}

// recordingState wraps a models.NexNodeState and records every
// RemoveWorkload and StoreWorkload call, so a test can assert a purge or a
// persist was (or was not) attempted. This matters because KV Purge of an
// already-missing key succeeds silently, so asserting on KV contents alone
// cannot distinguish "no purge attempted" from "purge attempted against the
// wrong key" -- the exact way the bug TestNodeUndeployUnconfirmedKeepsState
// pins was invisible. The StoreWorkload recording exists for the same
// reason on the write side: the UPDATE handler must not persist anything
// before it has validated the incoming definition (see
// node_update_workload_test.go).
type recordingState struct {
	models.NexNodeState

	mu          sync.Mutex
	removeCalls []removeWorkloadCall
	storeCalls  []storeWorkloadCall
}

func (r *recordingState) RemoveWorkload(workloadType, workloadId string) error {
	r.mu.Lock()
	r.removeCalls = append(r.removeCalls, removeWorkloadCall{workloadType: workloadType, workloadId: workloadId})
	r.mu.Unlock()
	return r.NexNodeState.RemoveWorkload(workloadType, workloadId)
}

func (r *recordingState) StoreWorkload(workloadId string, swr models.StartWorkloadRequest, expectedRevision uint64) error {
	r.mu.Lock()
	r.storeCalls = append(r.storeCalls, storeWorkloadCall{workloadId: workloadId, request: swr})
	r.mu.Unlock()
	return r.NexNodeState.StoreWorkload(workloadId, swr, expectedRevision)
}

func (r *recordingState) stores() []storeWorkloadCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]storeWorkloadCall(nil), r.storeCalls...)
}

func (r *recordingState) calls() []removeWorkloadCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]removeWorkloadCall(nil), r.removeCalls...)
}

// TestNodeUndeployUnconfirmedKeepsState pins the fix for a live bug in
// handleStopWorkload (handlers.go): an UNDEPLOY that no agent confirms must
// not purge persisted node state. Before the fix, the handler unconditionally
// called state.RemoveWorkload(ret.WorkloadType, workloadID) using a
// response-defaulted (zero-value) WorkloadType whenever no agent confirmed
// the stop -- silently purging the wrong KV key ("_<id>") while the real
// record ("<type>_<id>") survived to be resurrected by
// resume-on-registration. The fix purges only when the stop is confirmed
// (ret.Stopped == true).
func TestNodeUndeployUnconfirmedKeepsState(t *testing.T) {
	workDir := t.TempDir()
	s := _test.StartNatsServer(t, workDir)
	defer s.Shutdown()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Build the inmem agent ourselves (rather than letting StartNexus build
	// its default one) so the test retains a handle to flip FailStops after
	// the node is running. The node id must match what StartNexus derives
	// for node index 0 (Node1Seed/Node1Pub), since a single pre-built runner
	// is threaded straight through to that node.
	runner, inmemAgent, err := inmem.NewInMemAgentWithHandle("testnexus", _test.Node1Pub, logger)
	be.NilErr(t, err)

	// Open our own handle to the same KV bucket StartNexus's state=true path
	// will create ("nex-<node pubkey>"). NewNatsKVState is idempotent
	// against an already-existing bucket (falls back from ErrBucketExists to
	// KeyValue()), so it does not matter which of the two callers -- ours
	// here, or StartNexus's internal one below -- creates it first; both
	// end up bound to the identical durable bucket.
	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	bucketName := fmt.Sprintf("nex-%s", _test.Node1Pub)
	sKV, err := iState.NewNatsKVState(nc, bucketName, logger)
	be.NilErr(t, err)

	rec := &recordingState{NexNodeState: sKV}

	// state=true: first test in the repo to exercise StartNexus's real (NATS
	// KV backed) state wiring end-to-end, rather than the default no-state
	// path every other caller uses. extraOpts then overrides n.state with
	// our recording decorator (options apply in order, so this wins),
	// wrapping a handle to that same bucket.
	nexNodes := _test.StartNexusWithOptions(t, ctx, s.ClientURL(), 1, true, []nex.NexNodeOption{nex.WithState(rec)}, runner)
	be.Equal(t, 1, len(nexNodes))

	// AUCTION
	auctionReq := models.AuctionRequest{
		AgentType: "inmem",
		AuctionId: nuid.New().Next(),
	}
	auctionReqB, err := json.Marshal(auctionReq)
	be.NilErr(t, err)

	auctionRespRaw, err := nc.Request(models.AuctionRequestSubject(models.SystemNamespace), auctionReqB, time.Second*10)
	be.NilErr(t, err)

	auctionResp := models.AuctionResponse{}
	be.NilErr(t, json.Unmarshal(auctionRespRaw.Data, &auctionResp))
	be.Nonzero(t, auctionResp.BidderId)

	// ADEPLOY
	startWorkloadReq := models.StartWorkloadRequest{
		Description:       "test",
		Name:              "test",
		Namespace:         models.SystemNamespace,
		RunRequest:        "{}",
		WorkloadLifecycle: "service",
		WorkloadType:      "inmem",
	}
	startWorkloadReqB, err := json.Marshal(startWorkloadReq)
	be.NilErr(t, err)

	startWorkloadRespRaw, err := nc.Request(models.AuctionDeployRequestSubject(models.SystemNamespace, auctionResp.BidderId), startWorkloadReqB, time.Second*5)
	be.NilErr(t, err)

	startWorkloadResp := models.StartWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(startWorkloadRespRaw.Data, &startWorkloadResp))
	be.Nonzero(t, startWorkloadResp.Id)

	// Read the state KV bucket directly (bucket "nex-<node pubkey>", key
	// "<workload_type>_<workload_id>" -- see internal/state/nats_kv.go) and
	// confirm the deploy actually persisted a record under the real key.
	js, err := jetstream.New(nc)
	be.NilErr(t, err)
	kv, err := js.KeyValue(ctx, bucketName)
	be.NilErr(t, err)

	key := fmt.Sprintf("inmem_%s", startWorkloadResp.Id)
	_, err = kv.Get(ctx, key)
	be.NilErr(t, err)

	// Make the agent unable to confirm the stop, as if the nexlet were
	// down/unreachable/crashed.
	inmemAgent.FailStops = true

	stopReq := models.StopWorkloadRequest{Namespace: models.SystemNamespace}
	stopReqB, err := json.Marshal(stopReq)
	be.NilErr(t, err)

	stopRespRaw, err := nc.Request(models.UndeployRequestSubject(models.SystemNamespace, startWorkloadResp.Id), stopReqB, time.Second*5)
	be.NilErr(t, err)

	stopResp := models.StopWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(stopRespRaw.Data, &stopResp))
	be.False(t, stopResp.Stopped)

	// The record must still exist under the real key. Pre-fix this also
	// passes -- the old code purged the wrong key ("_<id>"), a no-op -- so
	// this assertion alone does not pin the bug; it is a sanity check.
	_, err = kv.Get(ctx, key)
	be.NilErr(t, err)

	// Load-bearing assertion: an unconfirmed stop must not attempt any purge
	// at all, correct key or not.
	be.Equal(t, 0, len(rec.calls()))

	// Now let the stop succeed.
	inmemAgent.FailStops = false

	stopRespRaw2, err := nc.Request(models.UndeployRequestSubject(models.SystemNamespace, startWorkloadResp.Id), stopReqB, time.Second*5)
	be.NilErr(t, err)

	stopResp2 := models.StopWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(stopRespRaw2.Data, &stopResp2))
	be.True(t, stopResp2.Stopped)

	calls := rec.calls()
	be.Equal(t, 1, len(calls))
	be.Equal(t, "inmem", calls[0].workloadType)
	be.Equal(t, startWorkloadResp.Id, calls[0].workloadId)

	_, err = kv.Get(ctx, key)
	be.Nonzero(t, err)
}
