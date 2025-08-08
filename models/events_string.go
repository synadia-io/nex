package models

func (NexNodeStartedEvent) String() string {
	return "NODESTARTED"
}

func (NexNodeStoppedEvent) String() string {
	return "NODESTOPPED"
}

func (NexNodeLameduckSetEvent) String() string {
	return "NODELAMEDUCKSET"
}

func (AgentStartedEvent) String() string {
	return "AGENTSTARTED"
}

func (AgentStoppedEvent) String() string {
	return "AGENTSTOPPED"
}

func (AgentLameduckSetEvent) String() string {
	return "AGENTLAMEDUCKSET"
}

func (WorkloadStartedEvent) String() string {
	return "WORKLOADSTARTED"
}

func (WorkloadStoppedEvent) String() string {
	return "WORKLOADSTOPPED"
}
