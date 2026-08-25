package nex_test

// External test package (nex_test), same rationale as
// node_update_workload_test.go: these tests need the updateHarness, which
// imports the root nex package.
//
// Handler-level tests for the purge-resurrect race between the replacement
// verbs and a confirmed UNDEPLOY. replaceWorkload's store-first write reads
// the record's revision immediately before storing; a record purged by a
// concurrent UNDEPLOY reads back as revision 0 (a purged key is
// indistinguishable from one that never existed), so the store takes the
// create-only path and SUCCEEDS over the purge tombstone -- persisting a
// record for a workload the operator was just told is undeployed. The verb's
// own stop then finds nothing running and answers updated:false "the stored
// definition will apply on the next agent registration" -- and it does:
// resume-on-registration resurrects the undeployed workload.
//
// The fix: when the store-first write CREATED the record (revision 0), an
// unconfirmed stop rolls that write back (a revision-checked delete, so a
// competing writer's newer record survives). A created record finishes no
// prior committed state, so there is nothing for resume to complete -- only
// something for it to wrongly resurrect.
//
// The interleaving is exact, not raced for: the UNDEPLOY runs synchronously
// inside the minter's one-shot pre-mint hook, which fires inside
// replaceWorkload after the ownership fetch and before the fresh revision
// read -- precisely where a real racing UNDEPLOY lands.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/synadia-io/nex/models"
)

// undeployConfirmed drives a full UNDEPLOY and asserts the node confirmed the
// stop -- the precondition for the purge whose tombstone the verbs race.
func (h *updateHarness) undeployConfirmed(t *testing.T, workloadID string) {
	t.Helper()

	stopReqB, err := json.Marshal(models.StopWorkloadRequest{Namespace: models.SystemNamespace})
	be.NilErr(t, err)

	respRaw, err := h.nc.Request(models.UndeployRequestSubject(models.SystemNamespace, workloadID), stopReqB, time.Second*20)
	be.NilErr(t, err)

	resp := models.StopWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(respRaw.Data, &resp))
	be.True(t, resp.Stopped)
}

// recordAbsent asserts no record exists under the workload's real KV key.
func (h *updateHarness) recordAbsent(t *testing.T, ctx context.Context, workloadType, workloadID string) {
	t.Helper()

	entry, err := h.kv.Get(ctx, workloadType+"_"+workloadID)
	if err == nil {
		rec := models.StartWorkloadRequest{}
		be.NilErr(t, json.Unmarshal(entry.Value(), &rec))
		t.Fatalf("record for %s_%s still on file (name %q); want absent", workloadType, workloadID, rec.Name)
	}
	be.True(t, err == jetstream.ErrKeyNotFound || err == jetstream.ErrKeyDeleted)
}

// An UPDATE that loses the race with a confirmed UNDEPLOY must not leave its
// definition on file: the operator was told the workload is stopped, and a
// surviving record is resurrected by the next agent registration.
func TestNodeUpdateRacingUndeployDoesNotResurrect(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)
	workloadID := h.deploy(t, serviceDef("v1", `{"command":"one"}`))

	// The UNDEPLOY lands inside the UPDATE, after its ownership fetch (the
	// workload is still running then) and before its fresh revision read
	// (which now sees the purge and reads revision 0).
	h.mint.injectBeforeMint(func() {
		h.undeployConfirmed(t, workloadID)
	})

	msg := h.update(t, models.SystemNamespace, workloadID, models.UpdateWorkloadRequest{
		Namespace:    models.SystemNamespace,
		StartRequest: serviceDef("v2-late-update", `{"command":"two"}`),
	})

	// The stop cannot confirm (nothing is running), so the update reports
	// failure through the verb's own response shape.
	be.Equal(t, "", msg.Header.Get("Nats-Service-Error-Code"))
	resp := models.UpdateWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(msg.Data, &resp))
	be.False(t, resp.Updated)

	// The decisive assertions: the created record was rolled back, and a
	// fresh agent registration resumes nothing under the undeployed id.
	h.recordAbsent(t, ctx, "inmem", workloadID)

	registerResp := h.registerAgain(t, "inmem", commandSchema)
	be.True(t, registerResp.Success)
	if _, resurrected := registerResp.ExistingState[workloadID]; resurrected {
		t.Fatalf("undeployed workload %s came back in resume state", workloadID)
	}
}

// RESTART's live-definition fallback takes the same create-only path
// (storedRevision 0 when no record is on file), so it must roll back the
// same way when its stop cannot confirm.
func TestNodeRestartRacingUndeployDoesNotResurrect(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)
	workloadID := h.deploy(t, serviceDef("v1", `{"command":"one"}`))

	// RESTART reads the stored record right after its ownership fetch; the
	// purge must land BEFORE that read so the read finds nothing and RESTART
	// takes the live-definition fallback -- the create-only (revision 0)
	// store this test pins the rollback of. A purge landing after the read
	// is the separate, already-covered CAS-conflict case.
	h.rec.injectBeforeGetRecord(func() {
		h.undeployConfirmed(t, workloadID)
	})
	reqB, err := json.Marshal(models.RestartWorkloadRequest{Namespace: models.SystemNamespace})
	be.NilErr(t, err)
	msg, err := h.nc.Request(models.RestartWorkloadRequestSubject(models.SystemNamespace, workloadID), reqB, time.Second*15)
	be.NilErr(t, err)

	be.Equal(t, "", msg.Header.Get("Nats-Service-Error-Code"))
	resp := models.UpdateWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(msg.Data, &resp))
	be.False(t, resp.Updated)

	h.recordAbsent(t, ctx, "inmem", workloadID)

	registerResp := h.registerAgain(t, "inmem", commandSchema)
	be.True(t, registerResp.Success)
	if _, resurrected := registerResp.ExistingState[workloadID]; resurrected {
		t.Fatalf("undeployed workload %s came back in resume state", workloadID)
	}
}
