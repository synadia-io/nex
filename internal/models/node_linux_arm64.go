package models

import controlapi "github.com/synadia-io/nex/control-api"

func GetNodeCapabilities(tags map[string]string) *controlapi.NodeCapabilities {
	return &controlapi.NodeCapabilities{
		Sandboxable: true,
		SupportedProviders: []controlapi.NexWorkload{
			controlapi.NexWorkloadNative,
			controlapi.NexWorkloadOCI,
			controlapi.NexWorkloadWasm,
		},
		NodeTags: tags,
	}
}
