package models

func GetNodeCapabilities() *NodeCapabilities {
	return &NodeCapabilities{
		Sandboxable: true,
		SupportedProviders: []NexExecutionProvider{
			NexExecutionProviderNative,
			NexExecutionProviderOCI,
			NexExecutionProviderWasm,
			NexExecutionProviderV8,
		},
	}
}
