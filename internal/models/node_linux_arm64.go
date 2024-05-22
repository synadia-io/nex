package models

func GetNodeCapabilities(tags map[string]string) *NodeCapabilities {
	return &NodeCapabilities{
		Sandboxable: true,
		SupportedProviders: []NexWorkload{
			NexWorkloadNative,
			NexWorkloadOCI,
			NexWorkloadWasm,
		},
		NodeTags: tags,
	}
}
