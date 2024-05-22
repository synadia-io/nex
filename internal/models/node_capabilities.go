package models

type NexWorkload string

const (
	NexWorkloadNative NexWorkload = "native"
	NexWorkloadV8     NexWorkload = "v8"
	NexWorkloadOCI    NexWorkload = "oci"
	NexWorkloadWasm   NexWorkload = "wasm"
)

type NodeCapabilities struct {
	Sandboxable        bool              `json:"sandboxable"`
	SupportedProviders []NexWorkload     `json:"supported_providers"`
	NodeTags           map[string]string `json:"node_tags"`
}
