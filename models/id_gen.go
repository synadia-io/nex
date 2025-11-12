package models

type IDGen interface {
	// Generate will create a new ID that is used for agents and workloads.
	// If the StartWorkloadRequest is nil, then the request is for an agent,
	// if its not nil, then the request is for a workload.
	Generate(startRequest *StartWorkloadRequest) string
}
