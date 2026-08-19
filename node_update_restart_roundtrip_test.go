package nex_test

// External test package (nex_test), same rationale as
// node_update_workload_test.go and node_restart_workload_test.go: this test
// reuses updateHarness (defined in node_update_workload_test.go), which
// already wires state=true, the real NATS-KV-backed state impl, a real
// per-mint signing minter, and every action/assertion helper (deploy,
// update, restart, storedRecord, agentDefinition, updateExpectSilence) the
// full lifecycle below needs.
//
// TestNodeUpdateRestartRoundtrip is Task 7 of the nex-workload-verbs plan:
// the canonical round-trip test (node_test.go TestNodeDeployCloneUndeploy)
// extended to the new UPDATE/RESTART verbs, and the first test in the repo
// to drive that full round trip -- deploy, update, restart, undeploy, then a
// post-undeploy update -- against the real state=true substrate end to end.
// The controller ruling that supersedes the original task-7 brief's step 5
// applies here: UPDATE addressed at an id no local nexlet holds (including
// one this test itself just undeployed) is a silent drop at the node (the
// CLONE convention -- handleUpdateWorkload), not an updated:false reply, so
// the last phase below asserts silence via updateExpectSilence rather than
// unmarshalling a response.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nkeys"

	"github.com/synadia-io/nex/models"
)

// namespacePing issues a raw WPING request for the given namespace and
// returns the parsed workload summaries.
func (h *updateHarness) namespacePing(t *testing.T, namespace string) models.AgentListWorkloadsResponse {
	t.Helper()

	req := models.AgentListWorkloadsRequest{Filter: []string{}, Namespace: namespace}
	reqB, err := json.Marshal(req)
	be.NilErr(t, err)

	msg, err := h.nc.Request(models.NamespacePingRequestSubject(namespace), reqB, time.Second*10)
	be.NilErr(t, err)

	resp := models.AgentListWorkloadsResponse{}
	be.NilErr(t, json.Unmarshal(msg.Data, &resp))
	return resp
}

// undeploy issues a raw UNDEPLOY control request and returns the parsed stop
// response.
func (h *updateHarness) undeploy(t *testing.T, namespace, workloadID string) models.StopWorkloadResponse {
	t.Helper()

	req := models.StopWorkloadRequest{Namespace: namespace}
	reqB, err := json.Marshal(req)
	be.NilErr(t, err)

	msg, err := h.nc.Request(models.UndeployRequestSubject(namespace, workloadID), reqB, time.Second*5)
	be.NilErr(t, err)

	resp := models.StopWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(msg.Data, &resp))
	return resp
}

func TestNodeUpdateRestartRoundtrip(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)

	// --- a: auction -> deploy definition A ---

	defA := serviceDef("v1", `{"command":"one"}`)
	workloadID := h.deploy(t, defA)

	pingResp := h.namespacePing(t, models.SystemNamespace)
	be.Equal(t, 1, len(pingResp))
	be.Equal(t, workloadID, pingResp[0].Id)
	be.Equal(t, "v1", pingResp[0].Name)

	storedA := h.storedRecord(t, ctx, "inmem", workloadID)
	be.Equal(t, "v1", storedA.Name)
	be.Equal(t, models.SystemNamespace, storedA.Namespace)
	be.Equal(t, `{"command":"one"}`, storedA.RunRequest)
	be.Equal(t, "inmem", storedA.WorkloadType)

	n1, ok := storedA.Metadata["nex_minted_nkey"].(string)
	be.True(t, ok)
	be.True(t, nkeys.IsValidPublicUserKey(n1))

	// --- b: UPDATE to definition B (changed run_request) ---

	defB := serviceDef("v2", `{"command":"two"}`)
	startSpy := h.spyOnStarts(t)

	updMsg := h.update(t, models.SystemNamespace, workloadID, models.UpdateWorkloadRequest{
		Namespace:    models.SystemNamespace,
		StartRequest: defB,
	})
	be.Equal(t, "", updMsg.Header.Get("Nats-Service-Error-Code"))

	updResp := models.UpdateWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(updMsg.Data, &updResp))
	be.Equal(t, workloadID, updResp.Id)
	be.True(t, updResp.Updated)

	liveAfterUpdate := h.agentDefinition(t, workloadID)
	be.Equal(t, "v2", liveAfterUpdate.Name)
	be.Equal(t, `{"command":"two"}`, liveAfterUpdate.RunRequest)

	storedB := h.storedRecord(t, ctx, "inmem", workloadID)
	be.Equal(t, "v2", storedB.Name)
	be.Equal(t, `{"command":"two"}`, storedB.RunRequest)
	be.Equal(t, "inmem", storedB.WorkloadType)

	n2, ok := storedB.Metadata["nex_minted_nkey"].(string)
	be.True(t, ok)
	be.True(t, nkeys.IsValidPublicUserKey(n2))
	be.True(t, n2 != n1)

	be.NilErr(t, h.nc.Flush())
	be.Equal(t, 1, startSpy.count())

	// --- c: RESTART (same stored definition, fresh credentials) ---

	restartSpy := h.spyOnStarts(t)

	restMsg := h.restart(t, models.SystemNamespace, workloadID, models.RestartWorkloadRequest{
		Namespace: models.SystemNamespace,
	})
	be.Equal(t, "", restMsg.Header.Get("Nats-Service-Error-Code"))

	restResp := models.UpdateWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(restMsg.Data, &restResp))
	be.Equal(t, workloadID, restResp.Id)
	be.True(t, restResp.Updated)

	liveAfterRestart := h.agentDefinition(t, workloadID)
	be.Equal(t, "v2", liveAfterRestart.Name)
	be.Equal(t, `{"command":"two"}`, liveAfterRestart.RunRequest)

	storedC := h.storedRecord(t, ctx, "inmem", workloadID)
	be.Equal(t, "v2", storedC.Name)
	be.Equal(t, `{"command":"two"}`, storedC.RunRequest)

	n3, ok := storedC.Metadata["nex_minted_nkey"].(string)
	be.True(t, ok)
	be.True(t, nkeys.IsValidPublicUserKey(n3))
	be.True(t, n3 != n2)
	be.True(t, n3 != n1)

	be.NilErr(t, h.nc.Flush())
	be.Equal(t, 1, restartSpy.count())

	// --- d: UNDEPLOY ---

	stopResp := h.undeploy(t, models.SystemNamespace, workloadID)
	be.Equal(t, workloadID, stopResp.Id)
	be.True(t, stopResp.Stopped)

	_, err := h.kv.Get(ctx, "inmem_"+workloadID)
	be.Nonzero(t, err)

	// --- e: UPDATE on the now-dead id -> silent drop ---
	//
	// Controller ruling: an UPDATE addressed at an unknown/dead workload id
	// is a silent drop at the node (the CLONE convention -- see
	// handleUpdateWorkload), never an updated:false reply. The undeployed
	// nexlet no longer holds workloadID (InMemAgent.StopWorkload removed
	// it), so the node's ownership lookup (GETWORKLOAD) fails and it never
	// answers at all: the caller must read no-responders/timeout as
	// not-found, exactly like TestNodeUpdateWorkloadUnknownID.
	preSilenceStores := len(h.rec.stores())

	h.updateExpectSilence(t, models.SystemNamespace, workloadID, models.UpdateWorkloadRequest{
		Namespace:    models.SystemNamespace,
		StartRequest: serviceDef("v3", `{"command":"three"}`),
	})

	be.Equal(t, preSilenceStores, len(h.rec.stores()))
}
