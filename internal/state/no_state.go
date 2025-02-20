package state

import "github.com/synadia-labs/nex/models"

var _ models.NexNodeState = (*NoState)(nil)

type NoState struct{}

func (n *NoState) StoreWorkload(workloadId string, nf models.StartWorkloadRequest) error {
	return nil
}

func (n *NoState) RemoveWorkload(workloadType, workloadId string) error {
	return nil
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
