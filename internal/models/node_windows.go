package models

import controlapi "github.com/synadia-io/nex/control-api"

func GetNodeCapabilities(tags map[string]string) *controlapi.NodeCapabilities {
	return &controlapi.NodeCapabilities{
		Sandboxable: false,
		SupportedProviders: []controlapi.NexWorkload{
			controlapi.NexWorkloadNative,
		},
		NodeTags: tags,
	}
}
