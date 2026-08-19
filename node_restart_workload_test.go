package nex_test

// External test package (nex_test), same rationale as
// node_update_workload_test.go. These are handler-level tests for the
// RESTART control verb. They reuse updateHarness (defined in
// node_update_workload_test.go) since RESTART shares the node/nexlet/state
// wiring UPDATE needs -- only the request shape and the "which definition
// gets replayed" question differ.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nuid"

	"github.com/synadia-io/nex/models"
)

// restart issues a raw RESTART control request and returns the reply message.
func (h *updateHarness) restart(t *testing.T, subjectNS, workloadID string, req models.RestartWorkloadRequest) *nats.Msg {
	t.Helper()

	reqB, err := json.Marshal(req)
	be.NilErr(t, err)

	// Generous timeout: like UPDATE, the ownership lookup inside the handler
	// waits on the nexlet's GETWORKLOAD reply.
	msg, err := h.nc.Request(models.RestartWorkloadRequestSubject(subjectNS, workloadID), reqB, time.Second*15)
	be.NilErr(t, err)
	return msg
}

// restartExpectSilence issues a raw RESTART control request that the node
// must drop without answering, and asserts nothing replied. See
// updateExpectSilence for why the timeout must exceed the handler's 3s
// ownership lookup.
func (h *updateHarness) restartExpectSilence(t *testing.T, subjectNS, workloadID string, req models.RestartWorkloadRequest) {
	t.Helper()

	reqB, err := json.Marshal(req)
	be.NilErr(t, err)

	_, err = h.nc.Request(models.RestartWorkloadRequestSubject(subjectNS, workloadID), reqB, time.Second*8)
	be.Nonzero(t, err)
	be.True(t, errors.Is(err, nats.ErrTimeout) || errors.Is(err, nats.ErrNoResponders))
}

// TestNodeRestartWorkloadNamespaceMismatch pins the subject/body namespace
// agreement check, exactly like UPDATE: it must reject before any lookup.
func TestNodeRestartWorkloadNamespaceMismatch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)

	msg := h.restart(t, "default", nuid.New().Next(), models.RestartWorkloadRequest{
		Namespace: "other",
	})

	be.Equal(t, models.ErrCodeForbidden, msg.Header.Get("Nats-Service-Error-Code"))
	be.Equal(t, 0, len(h.rec.stores()))
}

// TestNodeRestartWorkloadUnknownID pins the same silent-drop convention
// UPDATE uses for an id no local nexlet holds.
func TestNodeRestartWorkloadUnknownID(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)

	h.restartExpectSilence(t, models.SystemNamespace, nuid.New().Next(), models.RestartWorkloadRequest{
		Namespace: models.SystemNamespace,
	})

	be.Equal(t, 0, len(h.rec.stores()))
}

// TestNodeRestartWorkloadCrossNamespaceIsSilentlyDropped is RESTART's version
// of the ownership security test: a caller in one namespace must not be able
// to restart (and thereby re-mint credentials for, and briefly stop) a
// workload owned by another namespace.
func TestNodeRestartWorkloadCrossNamespaceIsSilentlyDropped(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)

	victimID := h.deploy(t, namespacedDef("tenant-a", "victim", `{"command":"one"}`))
	_ = h.deploy(t, namespacedDef("tenant-b", "attacker-owned", `{"command":"one"}`))

	startSpy := h.spyOnStarts(t)
	stopSpy := h.spyOnStops(t)

	h.restartExpectSilence(t, "tenant-b", victimID, models.RestartWorkloadRequest{
		Namespace: "tenant-b",
	})

	be.Equal(t, 2, len(h.rec.stores()))
	be.NilErr(t, h.nc.Flush())
	be.Equal(t, 0, stopSpy.count())
	be.Equal(t, 0, startSpy.count())

	stored := h.storedRecord(t, ctx, "inmem", victimID)
	be.Equal(t, "victim", stored.Name)
	be.Equal(t, "tenant-a", stored.Namespace)
}

// TestNodeRestartWorkloadReplaysStoredDefinition is the happy path: RESTART
// re-applies the STORED definition (not necessarily whatever the agent
// reports live -- see handleRestartWorkload's doc comment), rotating
// credentials in the process the same way UPDATE does. The definition's
// content is otherwise unchanged: RESTART's whole point is "same definition,
// fresh instance."
func TestNodeRestartWorkloadReplaysStoredDefinition(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	h := newUpdateHarness(t, ctx, commandSchema)
	workloadID := h.deploy(t, serviceDef("v1", `{"command":"one"}`))

	deployedNkey := h.storedRecord(t, ctx, "inmem", workloadID).Metadata["nex_minted_nkey"]
	be.Nonzero(t, deployedNkey)

	spy := h.spyOnStarts(t)

	msg := h.restart(t, models.SystemNamespace, workloadID, models.RestartWorkloadRequest{
		Namespace: models.SystemNamespace,
	})

	be.Equal(t, "", msg.Header.Get("Nats-Service-Error-Code"))

	resp := models.UpdateWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(msg.Data, &resp))
	be.Equal(t, workloadID, resp.Id)
	be.True(t, resp.Updated)

	// The stored definition's content is unchanged -- same name, same
	// run_request -- only the minted nkey rotates.
	stored := h.storedRecord(t, ctx, "inmem", workloadID)
	be.Equal(t, "v1", stored.Name)
	be.Equal(t, `{"command":"one"}`, stored.RunRequest)
	be.True(t, stored.Metadata["nex_minted_nkey"] != deployedNkey)

	be.NilErr(t, h.nc.Flush())
	be.Equal(t, 1, spy.count())

	live := h.agentDefinition(t, workloadID)
	be.Equal(t, "v1", live.Name)
	be.Equal(t, `{"command":"one"}`, live.RunRequest)
}
