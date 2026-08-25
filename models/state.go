package models

import "errors"

// ErrStateConflict is the sentinel a NexNodeState implementation returns
// when a StoreWorkload lost its compare-and-swap: the record it was writing
// against is not the record that is on file any more, because another writer
// got there first. Implementations wrap their backend's own conflict error
// with this (see internal/state/nats_kv.go), so callers match with
// errors.Is(err, models.ErrStateConflict) and never have to know what the
// backend calls it.
//
// A conflict is NOT a server fault. It means the caller's read is stale, and
// the only correct responses are to re-read and retry, or to leave the newer
// record alone -- never to overwrite it. Each of the node's three writers
// picks one of those (handlers.go); none of them may fall back to a blind
// write.
var ErrStateConflict = errors.New("workload record was modified concurrently")

// NexNodeState is the node's persisted view of which workloads it is
// supposed to be running. Exactly one record exists per workload, keyed by
// workload type and workload id, and three separate node paths write it: the
// deploy path (after responding to the caller), resume-on-registration's
// credential re-stamp, and the workload-replacement verbs' store-first
// write.
//
// Those writers overlap in time, so every write is a compare-and-swap
// against the revision the writer read. There is deliberately NO
// unconditional put in this interface: a blind write is how a definition
// gets silently reverted -- the resume path re-stamping a stale snapshot
// over a newly stored definition, or a deploy's late write undoing a fast
// update -- and leaving a blind put available is an invitation to
// reintroduce exactly that.
type NexNodeState interface {
	// StoreWorkload persists swr under (swr.WorkloadType, workloadId),
	// conditional on expectedRevision:
	//
	//   - expectedRevision == 0 means CREATE-ONLY: the write succeeds only
	//     if no record exists for that key. This is what a caller that has
	//     never read a record (the deploy path) and a caller whose read
	//     found nothing both pass.
	//   - expectedRevision > 0 means REPLACE-IF-UNCHANGED: the write
	//     succeeds only if the record on file is still at that revision.
	//
	// Either way, a lost race returns an error matching ErrStateConflict
	// and the stored record is left untouched. The revision to pass comes
	// from GetWorkloadRecord, which is why its not-found revision is 0: the
	// value round-trips into the create-only case without a special branch
	// at the call site.
	//
	// On success the revision of the written record is returned -- what a
	// writer that may have to undo its own write passes to
	// RemoveWorkloadAtRevision (the replacement verbs roll back a record
	// they CREATED when the stop they gate on is never confirmed, so a
	// workload a concurrent UNDEPLOY purged cannot be resurrected by their
	// leftover write).
	StoreWorkload(workloadId string, swr StartWorkloadRequest, expectedRevision uint64) (uint64, error)

	// RemoveWorkload deletes the record for (workloadType, workloadId). It
	// is unconditional: a purge is only ever issued for a stop the node has
	// already confirmed, so there is no stale read to guard against.
	RemoveWorkload(workloadType, workloadId string) error

	// RemoveWorkloadAtRevision deletes the record only while it is still at
	// revision -- the rollback half of StoreWorkload's returned revision. A
	// record that moved on belongs to a newer writer and is left untouched
	// (ErrStateConflict); a record already gone is success, because the only
	// caller is undoing its own create and "gone" is the desired end state.
	RemoveWorkloadAtRevision(workloadType, workloadId string, revision uint64) error

	// GetWorkloadRecord returns the stored definition for (workloadType,
	// workloadID) together with the revision to pass back to StoreWorkload.
	//
	// A missing record is NOT an error: it returns (nil, 0, nil). Absence
	// is an ordinary, reachable state -- the deploy path stores only after
	// it has already responded and does not fail the deploy if that store
	// errors, so a workload can legitimately be running with no record --
	// and callers must branch on it anyway. Returning a sentinel error
	// instead would force every caller through errors.Is before it could
	// even look at the revision, for a case that is not exceptional. A
	// non-nil error therefore always means the read itself failed.
	GetWorkloadRecord(workloadType, workloadID string) (*StartWorkloadRequest, uint64, error)

	// GetStateForAgent returns the state of the NexNode for a given agent in
	// the form of a map of workloadId to StartWorkloadRequest
	GetStateByAgent(agentName string) (map[string]StartWorkloadRequest, error)
	GetStateByNamespace(namespace string) (map[string]StartWorkloadRequest, error)
}
