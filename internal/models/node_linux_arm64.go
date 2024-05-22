package models

func GetNodeCapabilities(tags map[string]string) *NodeCapabilities {
	return &NodeCapabilities{
		Sandboxable: true,
		SupportedProviders: []NexExecutionProvider{
			NexExecutionProviderNative,
			NexExecutionProviderOCI,
			NexExecutionProviderWasm,
		},
		NodeTags: tags,
	}
}
