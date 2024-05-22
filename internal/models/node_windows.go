package models

func GetNodeCapabilities() *NodeCapabilities {
	return &NodeCapabilities{
		Sandboxable: false,
		SupportedProviders: []NexExecutionProvider{
			NexExecutionProviderNative,
		},
	}
}
