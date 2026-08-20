package nex_test

// External test package for the same import-cycle reason as
// node_update_workload_test.go, whose harness these tests reuse.
//
// Two concerns are pinned here, both found by running the real CLI against a
// real node and the native nexlet (PR #523 field report):
//
//  1. The node's stop-confirmation wait must outlast a LEGITIMATE slow stop.
//     A nexlet's stop is synchronous and honest -- the native nexlet's is
//     bounded at ~5.75s (5s grace, SIGKILL, 750ms confirm) -- but the node's
//     RequestMany inherited the NATS connection's default request timeout
//     (2s) because the node's lifetime context carries no deadline. Every
//     workload that does not die on the first signal made UNDEPLOY reply
//     Stopped:false and made UPDATE abort with "stop unconfirmed" -- while
//     the nexlet's already-dispatched stop went on to kill the workload
//     anyway: destroyed workload, nothing started in its place.
//
//  2. RESTART must work when no record is stored. A node without --state
//     (the default) persists nothing, and even a stateful node can hold a
//     workload with no record (deploy does not fail on a failed store).
//     Replying "nothing to restart" made the verb useless exactly where a
//     restart is most wanted; the live definition the ownership fetch
//     already returned is the honest thing to replay.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/carlmjohnson/be"

	"github.com/synadia-io/nex/models"
)

// TestNodeUndeploySlowStopConfirmed: a stop that takes longer than the NATS
// connection's default request timeout (2s) but is otherwise perfectly
// healthy must still be confirmed, not reported as Stopped:false.
func TestNodeUndeploySlowStopConfirmed(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)
	workloadID := h.deploy(t, serviceDef("slow-stop", `{"command":"one"}`))

	h.agent.StopDelay = 3 * time.Second

	resp := h.undeploy(t, models.SystemNamespace, workloadID)
	be.Equal(t, workloadID, resp.Id)
	be.True(t, resp.Stopped)

	// The confirmed stop also purged the record.
	_, err := h.kv.Get(ctx, "inmem_"+workloadID)
	be.Nonzero(t, err)
}

// TestNodeUpdateSlowStopConfirmed: same budget, on the UPDATE path. An
// aborted wait here is worse than on UNDEPLOY -- the stop is already
// dispatched and will finish, so "stop unconfirmed" leaves the workload
// killed with no replacement started.
func TestNodeUpdateSlowStopConfirmed(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)
	workloadID := h.deploy(t, serviceDef("v1", `{"command":"one"}`))

	h.agent.StopDelay = 3 * time.Second

	msg := h.update(t, models.SystemNamespace, workloadID, models.UpdateWorkloadRequest{
		Namespace:    models.SystemNamespace,
		StartRequest: serviceDef("v2", `{"command":"two"}`),
	})
	be.Equal(t, "", msg.Header.Get("Nats-Service-Error-Code"))

	resp := models.UpdateWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(msg.Data, &resp))
	be.Equal(t, workloadID, resp.Id)
	be.True(t, resp.Updated)

	// The replacement is the one running now.
	live := h.agentDefinition(t, workloadID)
	be.Equal(t, "v2", live.Name)
	be.Equal(t, `{"command":"two"}`, live.RunRequest)
}

// TestNodeRestartWithoutStoredRecordReplaysLiveDefinition: with nothing on
// file for the workload, RESTART replays the definition the owning nexlet is
// running -- and, as a side effect of replaceWorkload's store-first write,
// leaves that definition persisted where a record was missing.
func TestNodeRestartWithoutStoredRecordReplaysLiveDefinition(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)
	workloadID := h.deploy(t, serviceDef("v1", `{"command":"one"}`))

	// Erase the record: the state a stateless node is in for EVERY
	// workload, and a stateful one for a deploy whose store failed.
	be.NilErr(t, h.kv.Delete(ctx, "inmem_"+workloadID))

	spy := h.spyOnStarts(t)

	msg := h.restart(t, models.SystemNamespace, workloadID, models.RestartWorkloadRequest{
		Namespace: models.SystemNamespace,
	})
	be.Equal(t, "", msg.Header.Get("Nats-Service-Error-Code"))

	resp := models.UpdateWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(msg.Data, &resp))
	be.Equal(t, workloadID, resp.Id)
	be.True(t, resp.Updated)

	be.NilErr(t, h.nc.Flush())
	be.Equal(t, 1, spy.count())

	// Same definition, fresh instance -- and the record is back on file.
	live := h.agentDefinition(t, workloadID)
	be.Equal(t, "v1", live.Name)
	be.Equal(t, `{"command":"one"}`, live.RunRequest)

	stored := h.storedRecord(t, ctx, "inmem", workloadID)
	be.Equal(t, "v1", stored.Name)
	be.Nonzero(t, stored.Metadata["nex_minted_nkey"])
}
