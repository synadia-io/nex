package actors

import (
	"ergo.services/ergo/gen"
)

const (
	ergoNodeName = "nex@localhost" // FIXME -- this const also defined in node/node.go

	InternalNatsServerReadyName = gen.Atom("internal_nats_server_ready")
	PostInit                    = "post_init"
	AgentsReady                 = "agents_ready" // sent by internal nats server when ALL agents are ready to start
	AgentReady                  = "agent_ready"  // sent by individual agent to nats server when it is ready to start
)

var (
	InternalNatsServerReady = gen.Event{Name: InternalNatsServerReadyName, Node: ergoNodeName}
)

type InternalNatsServerReadyEvent struct {
	AgentCredentials []agentCredential
}
