package state

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/synadia-io/nex/models"
)

func startNatsServer(t testing.TB, workDir string) *server.Server {
	t.Helper()

	server := server.New(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  workDir,
	})

	server.Start()
	if !server.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats server failed to start")
	}

	return server
}

func getChanCount(t testing.TB, ch <-chan string) int {
	t.Helper()

	count := 0
	for range ch {
		count++
	}

	return count
}

func TestNewKVState(t *testing.T) {
	server := startNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)

	s, err := NewNatsKVState(nc, "test", nil)
	be.NilErr(t, err)

	jsCtx, err := jetstream.New(nc)
	be.NilErr(t, err)

	kv, err := jsCtx.KeyValue(context.TODO(), "test")
	be.NilErr(t, err)

	kl, err := kv.ListKeys(context.TODO())
	be.NilErr(t, err)
	be.Equal(t, 0, getChanCount(t, kl.Keys()))

	be.NilErr(t, s.StoreWorkload("workload1", models.StartWorkloadRequest{
		Description:       "foogoo",
		Name:              "foogoo",
		Namespace:         "goo",
		RunRequest:        "{}",
		WorkloadLifecycle: "service",
		WorkloadType:      "foo",
	}, 0))
	kl, err = kv.ListKeys(context.TODO())
	be.NilErr(t, err)
	be.Equal(t, 1, getChanCount(t, kl.Keys()))

	be.NilErr(t, s.StoreWorkload("workload2", models.StartWorkloadRequest{
		Description:       "goo",
		Name:              "goo",
		Namespace:         "goo",
		RunRequest:        "{}",
		WorkloadLifecycle: "service",
		WorkloadType:      "bar",
	}, 0))
	kl, err = kv.ListKeys(context.TODO())
	be.NilErr(t, err)
	be.Equal(t, 2, getChanCount(t, kl.Keys()))

	fooState, err := s.GetStateByAgent("foo")
	be.NilErr(t, err)
	be.Equal(t, 1, len(fooState))

	gooState, err := s.GetStateByNamespace("goo")
	be.NilErr(t, err)
	be.Equal(t, 2, len(gooState))

	be.NilErr(t, s.RemoveWorkload("foo", "workload1"))
	kl, err = kv.ListKeys(context.TODO())
	be.NilErr(t, err)
	be.Equal(t, 1, getChanCount(t, kl.Keys()))

	be.NilErr(t, s.RemoveWorkload("bar", "workload2"))
	kl, err = kv.ListKeys(context.TODO())
	be.NilErr(t, err)
	be.Equal(t, 0, getChanCount(t, kl.Keys()))
}

// workloadDef is a minimal, valid StartWorkloadRequest for the CAS tests
// below. Only Name and WorkloadType carry meaning here: WorkloadType is half
// the KV key ("<workload_type>_<workload_id>"), Name is what tells two
// otherwise identical definitions apart.
func workloadDef(name, workloadType string) models.StartWorkloadRequest {
	return models.StartWorkloadRequest{
		Description:       name,
		Name:              name,
		Namespace:         "ns",
		RunRequest:        "{}",
		WorkloadLifecycle: "service",
		WorkloadType:      workloadType,
	}
}

// TestKVStateGetWorkloadRecordRoundTrip pins the read half of the CAS
// contract: a missing record is reported as (nil, 0, nil) -- not an error --
// so a caller can feed the returned revision straight back into
// StoreWorkload, where 0 means create-only; and a present record comes back
// with a non-zero revision that advances on every successful write.
func TestKVStateGetWorkloadRecordRoundTrip(t *testing.T) {
	server := startNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)

	s, err := NewNatsKVState(nc, "test", nil)
	be.NilErr(t, err)

	// Absent: the not-found signal is a nil record with revision 0 and no
	// error. Callers branch on the nil record, and 0 is exactly the
	// "create-only" revision they then pass to StoreWorkload.
	rec, rev, err := s.GetWorkloadRecord("foo", "missing")
	be.NilErr(t, err)
	be.Zero(t, rec)
	be.Equal(t, uint64(0), rev)

	be.NilErr(t, s.StoreWorkload("workload1", workloadDef("v1", "foo"), 0))

	rec, rev, err = s.GetWorkloadRecord("foo", "workload1")
	be.NilErr(t, err)
	be.Nonzero(t, rec)
	be.Equal(t, "v1", rec.Name)
	be.True(t, rev > 0)

	// A successful CAS write advances the revision.
	be.NilErr(t, s.StoreWorkload("workload1", workloadDef("v2", "foo"), rev))

	rec2, rev2, err := s.GetWorkloadRecord("foo", "workload1")
	be.NilErr(t, err)
	be.Equal(t, "v2", rec2.Name)
	be.True(t, rev2 > rev)

	// The key is scoped by workload TYPE, so the same id under another type
	// is a different record -- this is what makes a type change write a
	// second key rather than replacing the first (see replaceWorkload).
	rec3, rev3, err := s.GetWorkloadRecord("bar", "workload1")
	be.NilErr(t, err)
	be.Zero(t, rec3)
	be.Equal(t, uint64(0), rev3)
}

// TestKVStateCreateOnlyRejectsExistingKey pins expectedRevision == 0: it is
// create-only, and must fail with models.ErrStateConflict when the record
// already exists rather than overwriting it. This is what makes the deploy
// path's post-response store unable to clobber a definition an UPDATE stored
// in the meantime (handleAuctionDeployWorkload).
func TestKVStateCreateOnlyRejectsExistingKey(t *testing.T) {
	server := startNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)

	s, err := NewNatsKVState(nc, "test", nil)
	be.NilErr(t, err)

	be.NilErr(t, s.StoreWorkload("workload1", workloadDef("winner", "foo"), 0))

	err = s.StoreWorkload("workload1", workloadDef("loser", "foo"), 0)
	be.Nonzero(t, err)
	be.True(t, errors.Is(err, models.ErrStateConflict))

	// The loser's definition did not land.
	rec, _, err := s.GetWorkloadRecord("foo", "workload1")
	be.NilErr(t, err)
	be.Equal(t, "winner", rec.Name)
}

// TestKVStateUpdateRejectsStaleRevision pins the compare-and-swap itself:
// two writers read the same revision, the first wins, and the second must be
// rejected with models.ErrStateConflict instead of silently reverting the
// first. That lost-update is the whole bug (bead s3n-7bf).
func TestKVStateUpdateRejectsStaleRevision(t *testing.T) {
	server := startNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)

	s, err := NewNatsKVState(nc, "test", nil)
	be.NilErr(t, err)

	be.NilErr(t, s.StoreWorkload("workload1", workloadDef("v1", "foo"), 0))

	// Both writers read the same revision -- the interleaving the blind Put
	// could not survive.
	_, revA, err := s.GetWorkloadRecord("foo", "workload1")
	be.NilErr(t, err)
	_, revB, err := s.GetWorkloadRecord("foo", "workload1")
	be.NilErr(t, err)
	be.Equal(t, revA, revB)

	be.NilErr(t, s.StoreWorkload("workload1", workloadDef("winner", "foo"), revA))

	err = s.StoreWorkload("workload1", workloadDef("loser", "foo"), revB)
	be.Nonzero(t, err)
	be.True(t, errors.Is(err, models.ErrStateConflict))

	rec, rev, err := s.GetWorkloadRecord("foo", "workload1")
	be.NilErr(t, err)
	be.Equal(t, "winner", rec.Name)

	// Re-reading yields the winner's revision, which the loser can retry
	// against -- the documented recovery for a conflict.
	be.NilErr(t, s.StoreWorkload("workload1", workloadDef("retried", "foo"), rev))
	rec, _, err = s.GetWorkloadRecord("foo", "workload1")
	be.NilErr(t, err)
	be.Equal(t, "retried", rec.Name)
}

// TestKVStateCreateOnlyAfterRemoveSucceeds pins that a purge really frees
// the key for a create-only write: UNDEPLOY purges the record
// (handleStopWorkload), and a later deploy that happens to reuse the key
// must not be permanently locked out by a tombstone.
func TestKVStateCreateOnlyAfterRemoveSucceeds(t *testing.T) {
	server := startNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)

	s, err := NewNatsKVState(nc, "test", nil)
	be.NilErr(t, err)

	be.NilErr(t, s.StoreWorkload("workload1", workloadDef("v1", "foo"), 0))
	be.NilErr(t, s.RemoveWorkload("foo", "workload1"))

	rec, rev, err := s.GetWorkloadRecord("foo", "workload1")
	be.NilErr(t, err)
	be.Zero(t, rec)
	be.Equal(t, uint64(0), rev)

	be.NilErr(t, s.StoreWorkload("workload1", workloadDef("v2", "foo"), 0))
	rec, _, err = s.GetWorkloadRecord("foo", "workload1")
	be.NilErr(t, err)
	be.Equal(t, "v2", rec.Name)
}

// TestKVStateGetStateByAgentPrefixIsExact pins that the agent-type match is
// on the whole key segment, not on a bare string prefix.
//
// The key is "<workload_type>_<workload_id>", so the only correct match for
// agent type "docker" is the prefix "docker_". Matching "docker" alone also
// swallows every key of every type that STARTS with it -- "dockerx_wl1" --
// and the TrimPrefix that follows uses "docker_", which does not match, so
// the foreign key comes back whole as the workload id.
//
// The damage is not confined to this function. Resume then re-reads each id
// under the REGISTERING type ("docker_dockerx_wl1"), finds nothing, and
// drops the record as removed -- so a "dockerx" workload silently fails to
// resume whenever a "docker" agent registers first, and the reason is
// invisible at the resume call site.
func TestKVStateGetStateByAgentPrefixIsExact(t *testing.T) {
	server := startNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)

	s, err := NewNatsKVState(nc, "test", nil)
	be.NilErr(t, err)

	// Two agent types where one is a string prefix of the other.
	be.NilErr(t, s.StoreWorkload("wl1", workloadDef("short", "docker"), 0))
	be.NilErr(t, s.StoreWorkload("wl2", workloadDef("long", "dockerx"), 0))

	shortState, err := s.GetStateByAgent("docker")
	be.NilErr(t, err)
	be.Equal(t, 1, len(shortState))
	rec, ok := shortState["wl1"]
	be.True(t, ok)
	be.Equal(t, "short", rec.Name)

	longState, err := s.GetStateByAgent("dockerx")
	be.NilErr(t, err)
	be.Equal(t, 1, len(longState))
	rec, ok = longState["wl2"]
	be.True(t, ok)
	be.Equal(t, "long", rec.Name)
}
