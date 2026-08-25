package state

import "github.com/synadia-io/nex/models"

var _ models.NexNodeState = (*NoState)(nil)

type NoState struct{}

// StoreWorkload discards the record. expectedRevision is ignored: nothing
// is stored, so nothing can conflict, and NoState therefore never returns
// models.ErrStateConflict. The returned revision is 0 -- there is no record
// a rollback could target.
func (n *NoState) StoreWorkload(workloadId string, nf models.StartWorkloadRequest, expectedRevision uint64) (uint64, error) {
	return 0, nil
}

func (n *NoState) RemoveWorkload(workloadType, workloadId string) error {
	return nil
}

// RemoveWorkloadAtRevision succeeds vacuously: nothing is ever stored, so the
// record is always already "gone", which the interface documents as success.
func (n *NoState) RemoveWorkloadAtRevision(workloadType, workloadId string, revision uint64) error {
	return nil
}

// GetWorkloadRecord always reports the not-found signal documented on
// models.NexNodeState -- (nil, 0, nil) -- because NoState stores nothing.
// Callers reading a record back therefore behave as they would against a
// node whose record was never written, which is the honest answer for a node
// running with state disabled.
func (n *NoState) GetWorkloadRecord(workloadType, workloadID string) (*models.StartWorkloadRequest, uint64, error) {
	return nil, 0, nil
}

// GetStateForAgent returns the state of the NexNode for a given agent in
// the form of a map of workloadId to StartWorkloadRequest
func (n *NoState) GetStateByAgent(agentName string) (map[string]models.StartWorkloadRequest, error) {
	ret := make(map[string]models.StartWorkloadRequest)
	return ret, nil
}

func (n *NoState) GetStateByNamespace(namespace string) (map[string]models.StartWorkloadRequest, error) {
	ret := make(map[string]models.StartWorkloadRequest)
	return ret, nil
}
