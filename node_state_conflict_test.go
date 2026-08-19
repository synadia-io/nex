package nex_test

// External test package (nex_test), same rationale as
// node_update_workload_test.go: these tests need
// _test.StartNexusWithOptions, which imports the root nex package.
//
// Handler-level tests for the workload-record lost-update race (bead
// s3n-7bf). Three node-side writers share one KV key
// ("<workload_type>_<workload_id>", internal/state/nats_kv.go) and used to
// reach it with a blind Put, so whichever wrote last won regardless of what
// it had read:
//
//  1. the deploy path's post-response store (handleAuctionDeployWorkload),
//     which can land AFTER a fast UPDATE has already replaced the record;
//  2. resume-on-registration's nkey re-stamp (handleRegisterAgent), which
//     snapshotted every record, minted per record, then wrote the SNAPSHOT
//     back -- silently reverting any definition stored in between;
//  3. UPDATE/RESTART's store-first write (replaceWorkload).
//
// All three now go through compare-and-swap (models.NexNodeState:
// GetWorkloadRecord returns a revision, StoreWorkload takes the revision it
// expects), so a writer that lost a race is told so instead of overwriting.
//
// The interleavings are made exact rather than raced for: recordingState's
// one-shot injection hooks perform the competing write synchronously at the
// precise point a real racing writer would have to land.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nkeys"
	"github.com/nats-io/nuid"

	nex "github.com/synadia-io/nex"
	"github.com/synadia-io/nex/models"
)

// fixedWorkloadIDGen hands out a caller-chosen id for every workload while
// leaving agent/bidder ids random (models.IDGen: a nil start request means
// "generate an agent id"). Deploy mints the workload id node-side, so
// without this a test cannot know the KV key a deploy is about to write --
// and therefore cannot plant a competing record there first.
type fixedWorkloadIDGen struct {
	workloadID string
}

func (g *fixedWorkloadIDGen) Generate(startRequest *models.StartWorkloadRequest) string {
	if startRequest == nil {
		return nuid.Next()
	}
	return g.workloadID
}

// TestNodeUpdateWorkloadConcurrentRecordChangeIsRejected pins race (3): the
// store-first write in replaceWorkload must not clobber a record that
// changed after the handler read it.
//
// Interleaving: UPDATE reads the record (revision R), a competing writer
// stores a different definition (revision R+1), then UPDATE tries to store
// against R. Before the fix that was a blind Put and the competing
// definition vanished with no trace and no signal to either caller; the
// UPDATE also reported updated:true, so the caller had every reason to
// believe its definition was live.
//
// After the fix the CAS fails, and because store-first runs BEFORE anything
// is stopped, the whole update aborts harmlessly: the competing definition
// survives, the running instance is untouched, and the caller is told to
// retry.
func TestNodeUpdateWorkloadConcurrentRecordChangeIsRejected(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)
	workloadID := h.deploy(t, serviceDef("v1", `{"command":"one"}`))

	// The competing write lands after replaceWorkload has read the record
	// and minted, immediately before its own store.
	h.rec.injectBeforeStore(func() {
		h.setStoredRecord(t, ctx, "inmem", workloadID, serviceDef("v2-concurrent", `{"command":"two"}`))
	})

	startSpy := h.spyOnStarts(t)
	stopSpy := h.spyOnStops(t)

	msg := h.update(t, models.SystemNamespace, workloadID, models.UpdateWorkloadRequest{
		Namespace:    models.SystemNamespace,
		StartRequest: serviceDef("v3-loser", `{"command":"three"}`),
	})

	// Not a server fault: a lost CAS is a legitimate outcome reported
	// through the verb's own response shape, like an unconfirmed stop.
	be.Equal(t, "", msg.Header.Get("Nats-Service-Error-Code"))

	resp := models.UpdateWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(msg.Data, &resp))
	be.Equal(t, workloadID, resp.Id)
	be.False(t, resp.Updated)
	be.Equal(t, "workload record was modified concurrently; retry", resp.Message)

	// The decisive assertion: the concurrent write SURVIVED. A blind Put
	// would have left "v3-loser" here.
	stored := h.storedRecord(t, ctx, "inmem", workloadID)
	be.Equal(t, "v2-concurrent", stored.Name)
	be.Equal(t, `{"command":"two"}`, stored.RunRequest)

	// The abort happens before the stop, so the running instance is
	// untouched -- no stop, no start, no dual-writer window.
	be.NilErr(t, h.nc.Flush())
	be.Equal(t, 0, stopSpy.count())
	be.Equal(t, 0, startSpy.count())
}

// TestNodeDeployDoesNotClobberConcurrentRecord pins race (2): the deploy
// path responds to the caller and only THEN persists the record, so a fast
// UPDATE issued against the id it just handed back can be undone by the
// deploy's own late write.
//
// The window is real rather than theoretical -- the deploy's store is the
// last thing handleAuctionDeployWorkload does, after a full agent start
// round-trip -- and it is exactly what blocks a control plane from issuing
// deploy-then-update sequences.
//
// The competing record is planted directly at the key the deploy will use
// (deterministic via nex.WithIDGenerator), which stands in for "an UPDATE
// already replaced this record": what matters to the deploy's store is only
// that the key is already occupied by someone else's newer definition.
// After the fix that store is create-only, so it fails instead of winning.
func TestNodeDeployDoesNotClobberConcurrentRecord(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	workloadID := nuid.Next()
	h := newUpdateHarnessWithOptions(t, ctx, commandSchema,
		nex.WithIDGenerator(&fixedWorkloadIDGen{workloadID: workloadID}))

	// Already on file when the deploy's late store finally runs.
	winner := serviceDef("v2-updated", `{"command":"two"}`)
	h.setStoredRecord(t, ctx, "inmem", workloadID, winner)

	// h.deploy waits for the store CALL, which recordingState records
	// whether or not the underlying CAS succeeds -- so the wait is still a
	// valid barrier for the conflicting case.
	deployedID := h.deploy(t, serviceDef("v1-deployed", `{"command":"one"}`))
	be.Equal(t, workloadID, deployedID)

	// The decisive assertion: the newer definition is still on file. A
	// blind Put would have reverted it to "v1-deployed".
	stored := h.storedRecord(t, ctx, "inmem", workloadID)
	be.Equal(t, "v2-updated", stored.Name)
	be.Equal(t, `{"command":"two"}`, stored.RunRequest)
}

// TestNodeResumeUsesFreshRecordNotSnapshot pins race (1), the silent-revert
// class the store-first design exists to kill.
//
// resume-on-registration walks every record for the registering agent type,
// re-mints a credential per record, and re-stamps the record with the new
// public nkey (so a future fencing revocation targets the live credential).
// It used to do that against the SNAPSHOT taken by GetStateByAgent: any
// definition stored between the snapshot and the per-record write was
// overwritten by the older definition carrying a fresh nkey -- and, worse,
// the agent was then resumed from that older definition, so the revert
// propagated to the live instance too.
//
// The fix re-reads each record immediately before minting and stamps the
// FRESH definition under compare-and-swap. That closes the revert AND makes
// resume start the newest stored definition, which is what an interrupted
// UPDATE needs.
//
// Interleaving: the competing definition is written immediately after the
// snapshot is produced, which is the whole window.
func TestNodeResumeUsesFreshRecordNotSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)
	workloadID := h.deploy(t, serviceDef("v1-snapshot", `{"command":"one"}`))

	deployedNkey := h.storedRecord(t, ctx, "inmem", workloadID).Metadata["nex_minted_nkey"]
	be.Nonzero(t, deployedNkey)

	h.rec.injectAfterAgentSnapshot(func() {
		h.setStoredRecord(t, ctx, "inmem", workloadID, serviceDef("v2-stored-after-snapshot", `{"command":"two"}`))
	})

	// A second registration of the same RegisterType is what drives resume;
	// the agent id in the subject is caller-chosen and unrelated to
	// workload identity (GetStateByAgent keys off RegisterType), so a
	// synthetic one exercises exactly the path a crash-restarted nexlet
	// hits. Same technique as node_metadata_nkey_test.go.
	registerResp := h.registerAgain(t, "inmem", commandSchema)
	be.True(t, registerResp.Success)

	existing, ok := registerResp.ExistingState[workloadID]
	be.True(t, ok)

	// The definition handed back to the agent is the one stored AFTER the
	// snapshot, not the snapshot's.
	be.Equal(t, "v2-stored-after-snapshot", existing.Request.Name)
	be.Equal(t, `{"command":"two"}`, existing.Request.RunRequest)

	// ... and the persisted record still holds that definition, now
	// carrying the freshly minted nkey rather than the deploy's.
	stored := h.storedRecord(t, ctx, "inmem", workloadID)
	be.Equal(t, "v2-stored-after-snapshot", stored.Name)

	resumeNkey, ok := stored.Metadata["nex_minted_nkey"].(string)
	be.True(t, ok)
	be.True(t, nkeys.IsValidPublicUserKey(resumeNkey))
	be.Equal(t, existing.WorkloadCreds.NatsUserNkey, resumeNkey)
	be.Unequal(t, deployedNkey, stored.Metadata["nex_minted_nkey"])
}

// TestNodeResumeRetriesStampOnConflict covers the second half of the resume
// fix: the re-read closes most of the window but not all of it -- minting a
// credential takes real time, and a writer can still land between the
// re-read and the CAS that follows it. Losing that CAS must not mean writing
// the losing definition anyway, and must not mean abandoning the stamp
// either: the record is re-read once more and the newest definition is
// stamped instead.
//
// Interleaving: the competing definition lands immediately before resume's
// store -- after resume has already read the record it is writing against --
// so the CAS is guaranteed to lose.
//
// Without the retry the persisted nkey would stay behind the credential the
// agent was actually handed, which is exactly the stale-fencing hazard the
// re-stamp exists to prevent.
func TestNodeResumeRetriesStampOnConflict(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)
	workloadID := h.deploy(t, serviceDef("v1", `{"command":"one"}`))

	deployedNkey := h.storedRecord(t, ctx, "inmem", workloadID).Metadata["nex_minted_nkey"]
	be.Nonzero(t, deployedNkey)

	// Fires on the next store, which is resume's -- after its re-read.
	h.rec.injectBeforeStore(func() {
		h.setStoredRecord(t, ctx, "inmem", workloadID, serviceDef("v2-won-the-cas", `{"command":"two"}`))
	})

	registerResp := h.registerAgain(t, "inmem", commandSchema)
	be.True(t, registerResp.Success)

	existing, ok := registerResp.ExistingState[workloadID]
	be.True(t, ok)

	// The retry re-read, so the stamp landed on the winner's definition --
	// not on the one resume had already read and lost with.
	stored := h.storedRecord(t, ctx, "inmem", workloadID)
	be.Equal(t, "v2-won-the-cas", stored.Name)
	be.Equal(t, `{"command":"two"}`, stored.RunRequest)
	be.Equal(t, "v2-won-the-cas", existing.Request.Name)

	// ... and the persisted nkey is the live one, which is the whole point
	// of retrying rather than abandoning the stamp.
	resumeNkey, ok := stored.Metadata["nex_minted_nkey"].(string)
	be.True(t, ok)
	be.True(t, nkeys.IsValidPublicUserKey(resumeNkey))
	be.Equal(t, existing.WorkloadCreds.NatsUserNkey, resumeNkey)
	be.Unequal(t, deployedNkey, stored.Metadata["nex_minted_nkey"])
}

// registerAgain drives a synthetic agent registration of the given
// RegisterType against the harness's node and returns the response, which
// carries the resume state (RegisterAgentResponse.ExistingState).
func (h *updateHarness) registerAgain(t *testing.T, registerType, schema string) models.RegisterAgentResponse {
	t.Helper()

	xkp, err := nkeys.CreateCurveKeys()
	be.NilErr(t, err)
	xkPub, err := xkp.PublicKey()
	be.NilErr(t, err)

	reqB, err := json.Marshal(models.RegisterAgentRequest{
		Description:         "resume test agent",
		Name:                registerType + "-resume",
		PublicXkey:          xkPub,
		RegisterType:        registerType,
		StartRequestSchema:  schema,
		SupportedLifecycles: []models.WorkloadLifecycle{models.WorkloadLifecycleService},
		Version:             "0.0.0",
	})
	be.NilErr(t, err)

	respRaw, err := h.nc.Request(models.AgentAPIRegisterRequestSubject(nuid.Next(), h.nodePK), reqB, time.Second*10)
	be.NilErr(t, err)

	resp := models.RegisterAgentResponse{}
	be.NilErr(t, json.Unmarshal(respRaw.Data, &resp))
	return resp
}
