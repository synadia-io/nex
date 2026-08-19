package nex_test

// External test package (nex_test), same rationale as
// node_undeploy_state_test.go and node_metadata_nkey_test.go: these tests
// need _test.StartNexusWithOptions, which imports the root nex package, so
// an internal (package nex) test file would create an import cycle.
//
// These are handler-level tests for the UPDATE control verb. They exercise
// the node handler over real NATS against the in-memory nexlet; the full
// client-facing roundtrip is a separate task.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
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

// commandSchema is a deliberately restrictive run_request schema. The inmem
// nexlet's default schema is "{}" (accepts anything), which would make every
// assertion about the node REJECTING a run_request vacuous.
const commandSchema = `{
	"type": "object",
	"properties": {"command": {"type": "string"}},
	"required": ["command"],
	"additionalProperties": false
}`

// updateHarness is one NATS server + one nex node + one inmem nexlet, with
// real (NATS KV) node state wrapped in the recordingState decorator so tests
// can observe StoreWorkload/RemoveWorkload calls, and a real signing minter
// so the minted nkey stamped into the stored record is a genuine, distinct
// key per mint.
type updateHarness struct {
	nc     *nats.Conn
	kv     jetstream.KeyValue
	rec    *recordingState
	agent  *inmem.InMemAgent
	nodePK string
}

func newUpdateHarness(t *testing.T, ctx context.Context, schema string) *updateHarness {
	t.Helper()

	workDir := t.TempDir()
	s := _test.StartNatsServer(t, workDir)
	t.Cleanup(s.Shutdown)

	logger := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Built here rather than letting StartNexus build its default agent so
	// the test keeps a handle for FailStops and can register a restrictive
	// run_request schema. A single pre-built runner is threaded to node
	// index 0, whose keypair StartNexus derives from Node1Seed.
	runner, inmemAgent, err := inmem.NewInMemAgentWithHandle("testnexus", _test.Node1Pub, logger, inmem.WithStartRequestSchema(schema))
	be.NilErr(t, err)

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	t.Cleanup(nc.Close)

	bucketName := fmt.Sprintf("nex-%s", _test.Node1Pub)
	sKV, err := iState.NewNatsKVState(nc, bucketName, logger)
	be.NilErr(t, err)

	rec := &recordingState{NexNodeState: sKV}

	nexNodes := _test.StartNexusWithOptions(t, ctx, s.ClientURL(), 1, true,
		[]nex.NexNodeOption{
			nex.WithState(rec),
			nex.WithMinter(newTestSigningMinter(t, s.ClientURL(), _test.Node1Pub)),
		}, runner)
	be.Equal(t, 1, len(nexNodes))

	js, err := jetstream.New(nc)
	be.NilErr(t, err)
	kv, err := js.KeyValue(ctx, bucketName)
	be.NilErr(t, err)

	return &updateHarness{nc: nc, kv: kv, rec: rec, agent: inmemAgent, nodePK: _test.Node1Pub}
}

// deploy runs AUCTION + ADEPLOY and returns the minted workload id, waiting
// until the node has persisted the record (the deploy path stores AFTER
// responding to the caller, so the response alone does not imply a Put).
func (h *updateHarness) deploy(t *testing.T, def models.StartWorkloadRequest) string {
	t.Helper()

	// Captured before the request goes out: the Put can land at any point
	// after the ADEPLOY response, so sampling afterwards would sometimes
	// already include it and wait forever for one more.
	want := len(h.rec.stores()) + 1

	auctionReqB, err := json.Marshal(models.AuctionRequest{AgentType: def.WorkloadType, AuctionId: nuid.New().Next()})
	be.NilErr(t, err)

	auctionRespRaw, err := h.nc.Request(models.AuctionRequestSubject(models.SystemNamespace), auctionReqB, time.Second*10)
	be.NilErr(t, err)

	auctionResp := models.AuctionResponse{}
	be.NilErr(t, json.Unmarshal(auctionRespRaw.Data, &auctionResp))
	be.Nonzero(t, auctionResp.BidderId)

	defB, err := json.Marshal(def)
	be.NilErr(t, err)

	startRespRaw, err := h.nc.Request(models.AuctionDeployRequestSubject(models.SystemNamespace, auctionResp.BidderId), defB, time.Second*10)
	be.NilErr(t, err)

	startResp := models.StartWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(startRespRaw.Data, &startResp))
	be.Nonzero(t, startResp.Id)

	_test.WaitFor(t, time.Second*10, func() bool { return len(h.rec.stores()) == want }, "deploy to persist workload record")

	return startResp.Id
}

// update issues a raw UPDATE control request and returns the reply message.
func (h *updateHarness) update(t *testing.T, subjectNS, workloadID string, req models.UpdateWorkloadRequest) *nats.Msg {
	t.Helper()

	reqB, err := json.Marshal(req)
	be.NilErr(t, err)

	// Generous timeout: the ownership lookup inside the handler waits on the
	// nexlet's GETWORKLOAD reply.
	msg, err := h.nc.Request(models.UpdateWorkloadRequestSubject(subjectNS, workloadID), reqB, time.Second*15)
	be.NilErr(t, err)
	return msg
}

// updateExpectSilence issues a raw UPDATE control request that the node must
// drop without answering, and asserts nothing replied.
//
// The client timeout has to exceed the handler's own 3s ownership lookup,
// or a handler that DID intend to answer would look identical to one that
// dropped -- the assertion would pass for the wrong reason.
func (h *updateHarness) updateExpectSilence(t *testing.T, subjectNS, workloadID string, req models.UpdateWorkloadRequest) {
	t.Helper()

	reqB, err := json.Marshal(req)
	be.NilErr(t, err)

	_, err = h.nc.Request(models.UpdateWorkloadRequestSubject(subjectNS, workloadID), reqB, time.Second*8)
	be.Nonzero(t, err)
	be.True(t, errors.Is(err, nats.ErrTimeout) || errors.Is(err, nats.ErrNoResponders))
}

// storedRecord reads the node's persisted definition straight out of the KV
// bucket (key "<workload_type>_<workload_id>", see internal/state/nats_kv.go).
func (h *updateHarness) storedRecord(t *testing.T, ctx context.Context, workloadType, workloadID string) models.StartWorkloadRequest {
	t.Helper()

	entry, err := h.kv.Get(ctx, fmt.Sprintf("%s_%s", workloadType, workloadID))
	be.NilErr(t, err)

	rec := models.StartWorkloadRequest{}
	be.NilErr(t, json.Unmarshal(entry.Value(), &rec))
	return rec
}

// agentSpy counts agent-directed requests the node issues. It is a plain
// (non queue-group) subscription, so it observes a copy of every such
// message without intercepting it.
type agentSpy struct {
	mu sync.Mutex
	n  int
}

func (h *updateHarness) spyOn(t *testing.T, subject string) *agentSpy {
	t.Helper()

	spy := &agentSpy{}
	sub, err := h.nc.Subscribe(subject, func(_ *nats.Msg) {
		spy.mu.Lock()
		spy.n++
		spy.mu.Unlock()
	})
	be.NilErr(t, err)
	be.NilErr(t, h.nc.Flush())

	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return spy
}

func (h *updateHarness) spyOnStarts(t *testing.T) *agentSpy {
	t.Helper()
	return h.spyOn(t, models.AgentAPIStartWorkloadSubscribeSubject(h.nodePK, "*"))
}

func (h *updateHarness) spyOnStops(t *testing.T) *agentSpy {
	t.Helper()
	return h.spyOn(t, models.AgentAPIStopWorkloadSubscribeSubject(h.nodePK))
}

func (s *agentSpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

func serviceDef(name, runRequest string) models.StartWorkloadRequest {
	return namespacedDef(models.SystemNamespace, name, runRequest)
}

func namespacedDef(namespace, name, runRequest string) models.StartWorkloadRequest {
	return models.StartWorkloadRequest{
		Description:       name,
		Name:              name,
		Namespace:         namespace,
		RunRequest:        runRequest,
		WorkloadLifecycle: "service",
		WorkloadType:      "inmem",
	}
}

// TestNodeUpdateWorkloadNamespaceMismatch pins the subject/body namespace
// agreement check: it must reject before any lookup, mint or store, exactly
// like every other control verb (handlers.go).
func TestNodeUpdateWorkloadNamespaceMismatch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)

	// Subject namespace "default" is not "system", so the system-namespace
	// bypass does not apply and the body's "other" must be rejected.
	msg := h.update(t, "default", nuid.New().Next(), models.UpdateWorkloadRequest{
		Namespace:    "other",
		StartRequest: serviceDef("v2", `{"command":"two"}`),
	})

	be.Equal(t, models.ErrCodeForbidden, msg.Header.Get("Nats-Service-Error-Code"))
	be.Equal(t, 0, len(h.rec.stores()))
}

// TestNodeUpdateWorkloadInvalidRunRequestDoesNotStore pins the ordering
// requirement that validation precedes persistence: a run_request that
// violates the nexlet's registered schema must be rejected with a bad
// request AND must leave the persisted definition untouched. Asserting on
// the error alone would not catch a handler that stores first and validates
// second.
func TestNodeUpdateWorkloadInvalidRunRequestDoesNotStore(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)
	workloadID := h.deploy(t, serviceDef("v1", `{"command":"one"}`))

	spy := h.spyOnStarts(t)

	msg := h.update(t, models.SystemNamespace, workloadID, models.UpdateWorkloadRequest{
		Namespace:    models.SystemNamespace,
		StartRequest: serviceDef("v2", `{"nope":true}`),
	})

	be.Equal(t, models.ErrCodeBadRequest, msg.Header.Get("Nats-Service-Error-Code"))

	// Exactly the one store from the deploy: the rejected update added none.
	be.Equal(t, 1, len(h.rec.stores()))
	be.Equal(t, "v1", h.storedRecord(t, ctx, "inmem", workloadID).Name)
	be.Equal(t, 0, spy.count())
}

// TestNodeUpdateWorkloadUnknownID pins the convention for an UPDATE
// addressed at a workload no local nexlet holds: the node drops it
// silently, exactly as it drops a workload owned by another namespace
// (TestNodeUpdateWorkloadCrossNamespaceIsSilentlyDropped).
//
// The two cases MUST behave identically. Every node sees every control
// message, so a caller can count replies; if unknown ids answered and
// not-yours ids stayed silent, that count would tell the caller which of
// the two it hit -- an existence oracle for workloads it may not see. The
// cost is that callers cannot distinguish not-found from unreachable and
// must read no-responders/timeout as not-found, which is already what
// CLONE requires of them.
func TestNodeUpdateWorkloadUnknownID(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)

	h.updateExpectSilence(t, models.SystemNamespace, nuid.New().Next(), models.UpdateWorkloadRequest{
		Namespace:    models.SystemNamespace,
		StartRequest: serviceDef("v2", `{"command":"two"}`),
	})

	be.Equal(t, 0, len(h.rec.stores()))
}

// TestNodeUpdateWorkloadCrossNamespaceIsSilentlyDropped is the security
// test for the ownership check. The nexlet's lookup by workload id spans
// every namespace, so the node -- not the nexlet -- is what stops a caller
// in one namespace from replacing the definition of a workload owned by
// another. Two workloads in two namespaces make the drop meaningful: the
// caller's own namespace is populated, so silence cannot be explained away
// as "this node runs nothing for you".
func TestNodeUpdateWorkloadCrossNamespaceIsSilentlyDropped(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)

	victimID := h.deploy(t, namespacedDef("tenant-a", "victim", `{"command":"one"}`))
	_ = h.deploy(t, namespacedDef("tenant-b", "attacker-owned", `{"command":"one"}`))

	startSpy := h.spyOnStarts(t)
	stopSpy := h.spyOnStops(t)

	// tenant-b addresses tenant-a's workload. Subject and body agree, so
	// the first namespace check passes; only the fetched definition's
	// namespace exposes the mismatch.
	h.updateExpectSilence(t, "tenant-b", victimID, models.UpdateWorkloadRequest{
		Namespace:    "tenant-b",
		StartRequest: namespacedDef("tenant-b", "hijacked", `{"command":"two"}`),
	})

	// Untouched: no persist, no stop, no start, and the victim's stored
	// definition is still its own.
	be.Equal(t, 2, len(h.rec.stores()))
	be.Equal(t, 0, len(h.rec.calls()))
	be.NilErr(t, h.nc.Flush())
	be.Equal(t, 0, stopSpy.count())
	be.Equal(t, 0, startSpy.count())

	stored := h.storedRecord(t, ctx, "inmem", victimID)
	be.Equal(t, "victim", stored.Name)
	be.Equal(t, "tenant-a", stored.Namespace)
}

// TestNodeUpdateWorkloadTypeChangeRejected pins the rejection of a
// workload-type change.
//
// A type change cannot be done safely by this verb. The state key embeds
// the workload type, so the replacement would land on a NEW key beside the
// surviving old one, and resume-on-registration is scoped per agent type
// (handleRegisterAgent -> GetStateByAgent) -- so the two records get
// resumed by two different nexlets with no knowledge of each other. Let the
// stop go unconfirmed and both instances run: the dual-writer window this
// verb exists to close, reopened by the verb itself. Purging the old key
// first only narrows it, and adds a window where the workload is lost.
//
// So the rejection must land before anything is persisted, stopped or
// started -- which is what the assertions below check, not just the 403.
func TestNodeUpdateWorkloadTypeChangeRejected(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)
	workloadID := h.deploy(t, serviceDef("v1", `{"command":"one"}`))

	startSpy := h.spyOnStarts(t)
	stopSpy := h.spyOnStops(t)

	newType := serviceDef("v2", `{"command":"two"}`)
	newType.WorkloadType = "other"

	msg := h.update(t, models.SystemNamespace, workloadID, models.UpdateWorkloadRequest{
		Namespace:    models.SystemNamespace,
		StartRequest: newType,
	})

	be.Equal(t, models.ErrCodeForbidden, msg.Header.Get("Nats-Service-Error-Code"))
	be.Equal(t, "update cannot change a workload's type; undeploy and deploy instead", msg.Header.Get("Nats-Service-Error"))

	// Nothing happened: only the deploy's store, no purge of the old-type
	// key, no stop, no start.
	be.Equal(t, 1, len(h.rec.stores()))
	be.Equal(t, 0, len(h.rec.calls()))
	be.NilErr(t, h.nc.Flush())
	be.Equal(t, 0, stopSpy.count())
	be.Equal(t, 0, startSpy.count())

	stored := h.storedRecord(t, ctx, "inmem", workloadID)
	be.Equal(t, "v1", stored.Name)
	be.Equal(t, "inmem", stored.WorkloadType)
}

// TestNodeUpdateWorkloadStopUnconfirmedKeepsNewDefinition is the store-first
// test. With a nexlet that cannot confirm the stop, the update must abort
// WITHOUT starting anything (starting before a confirmed stop is the
// dual-writer bug this verb exists to remove), and the newly persisted
// definition must already be the NEW one -- that is the crash-safety
// property: whatever happens next, resume-on-registration completes the
// update rather than reverting it.
func TestNodeUpdateWorkloadStopUnconfirmedKeepsNewDefinition(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)
	workloadID := h.deploy(t, serviceDef("v1", `{"command":"one"}`))

	deployedNkey := h.storedRecord(t, ctx, "inmem", workloadID).Metadata["nex_minted_nkey"]
	be.Nonzero(t, deployedNkey)

	spy := h.spyOnStarts(t)
	h.agent.FailStops = true

	msg := h.update(t, models.SystemNamespace, workloadID, models.UpdateWorkloadRequest{
		Namespace:    models.SystemNamespace,
		StartRequest: serviceDef("v2", `{"command":"two"}`),
	})

	be.Equal(t, "", msg.Header.Get("Nats-Service-Error-Code"))

	resp := models.UpdateWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(msg.Data, &resp))
	be.Equal(t, workloadID, resp.Id)
	be.False(t, resp.Updated)
	be.True(t, strings.Contains(resp.Message, "stop unconfirmed"))

	// Store-first, observable: the new definition is already persisted even
	// though the replacement never completed.
	stored := h.storedRecord(t, ctx, "inmem", workloadID)
	be.Equal(t, "v2", stored.Name)
	be.Equal(t, `{"command":"two"}`, stored.RunRequest)

	// Freshly minted creds, stamped before the store (so the stored nkey is
	// the one the replacement container would have received).
	be.Nonzero(t, stored.Metadata["nex_minted_nkey"])
	be.True(t, stored.Metadata["nex_minted_nkey"] != deployedNkey)

	// No start was issued, and the nexlet still holds the old definition.
	be.NilErr(t, h.nc.Flush())
	be.Equal(t, 0, spy.count())
	be.Equal(t, "v1", h.agentDefinition(t, workloadID).Name)
}

// TestNodeUpdateWorkloadReplacesDefinition is the happy path: same workload
// id, new definition persisted, old instance stopped and a new one started
// from the new definition with freshly minted credentials.
func TestNodeUpdateWorkloadReplacesDefinition(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)
	workloadID := h.deploy(t, serviceDef("v1", `{"command":"one"}`))

	deployedNkey := h.storedRecord(t, ctx, "inmem", workloadID).Metadata["nex_minted_nkey"]
	spy := h.spyOnStarts(t)

	msg := h.update(t, models.SystemNamespace, workloadID, models.UpdateWorkloadRequest{
		Namespace:    models.SystemNamespace,
		StartRequest: serviceDef("v2", `{"command":"two"}`),
	})

	be.Equal(t, "", msg.Header.Get("Nats-Service-Error-Code"))

	resp := models.UpdateWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(msg.Data, &resp))
	be.Equal(t, workloadID, resp.Id)
	be.True(t, resp.Updated)

	stored := h.storedRecord(t, ctx, "inmem", workloadID)
	be.Equal(t, "v2", stored.Name)
	be.Equal(t, `{"command":"two"}`, stored.RunRequest)
	be.True(t, stored.Metadata["nex_minted_nkey"] != deployedNkey)

	// Exactly one replacement start, against the SAME workload id.
	be.NilErr(t, h.nc.Flush())
	be.Equal(t, 1, spy.count())

	live := h.agentDefinition(t, workloadID)
	be.Equal(t, "v2", live.Name)
	be.Equal(t, `{"command":"two"}`, live.RunRequest)
}

// agentDefinition asks the nexlet directly for the definition it currently
// holds for workloadID.
func (h *updateHarness) agentDefinition(t *testing.T, workloadID string) models.StartWorkloadRequest {
	t.Helper()

	msg, err := h.nc.Request(models.AgentAPIGetWorkloadRequestSubject(h.nodePK, workloadID), nil, time.Second*5)
	be.NilErr(t, err)

	def := models.StartWorkloadRequest{}
	be.NilErr(t, json.Unmarshal(msg.Data, &def))
	return def
}
