package models

func GetNodeCapabilities(tags map[string]string) *NodeCapabilities {
	return &NodeCapabilities{
		Sandboxable: false,
		SupportedProviders: []NexExecutionProvider{
			NexExecutionProviderNative,
			NexExecutionProviderOCI,
			NexExecutionProviderWasm,
		},
		NodeTags: tags,
	}
}
