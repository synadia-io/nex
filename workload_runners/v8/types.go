package main

const (
	NexEnvWorkloadId       = "NEX_WORKLOADID"
	NexEnvNodeNatsHost     = "NEX_NODE_NATS_HOST"
	NexEnvNodeNatsPort     = "NEX_NODE_NATS_PORT"
	NexEnvNodeNatsNkeySeed = "NEX_NODE_NATS_NKEY_SEED"
)

type WorkloadInfo struct {
	VmID         string `json:"vm_id"`
	ArtifactPath string `json:"artifact_path"`
	Namespace    string `json:"namespace"`
	WorkloadName string `json:"workload_name"`
}
