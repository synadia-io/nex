package models

import "errors"

var ErrLameduckShutdown error = errors.New("node shutdown due to lameduck mode")

const (
	SystemNamespace = "system"

	TagOS       = "nex.os"
	TagArch     = "nex.arch"
	TagCPUs     = "nex.cpucount"
	TagLameDuck = "nex.lameduck"
	TagNexus    = "nex.nexus"
	TagNodeName = "nex.node"

	AgentEnvNatsUrl = "NEX_AGENT_NATS_URL"
	AgentEnvNodeId  = "NEX_AGENT_NODE_ID"
)

var ReservedTagPrefixes = []string{"nex."}
