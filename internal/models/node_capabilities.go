package models

type NexExecutionProvider string

const (
	NexExecutionProviderNative NexExecutionProvider = "native"
	NexExecutionProviderV8     NexExecutionProvider = "v8"
	NexExecutionProviderOCI    NexExecutionProvider = "oci"
	NexExecutionProviderWasm   NexExecutionProvider = "wasm"
)

type NodeCapabilities struct {
	Sandboxable        bool                   `json:"sandboxable"`
	SupportedProviders []NexExecutionProvider `json:"supported_providers"`
	NodeTags           map[string]string      `json:"node_tags"`
}
