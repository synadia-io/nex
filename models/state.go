package models

type NexNodeState interface {
	StoreWorkload(workloadId string, swr StartWorkloadRequest) error
	RemoveWorkload(workloadType, workloadId string) error
	// GetStateForAgent returns the state of the NexNode for a given agent in
	// the form of a map of workloadId to StartWorkloadRequest
	GetStateByAgent(agentName string) (map[string]StartWorkloadRequest, error)
	GetStateByNamespace(namespace string) (map[string]StartWorkloadRequest, error)
}
