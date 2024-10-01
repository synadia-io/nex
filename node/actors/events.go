package actors

import (
	"ergo.services/ergo/gen"
)

const (
	ergoNodeName = "nex@localhost" // FIXME -- this const also defined in node/node.go

	InternalNatsServerReadyName = gen.Atom("internal_nats_server_ready")
	PostInit                    = "post_init"
)

var (
	InternalNatsServerReady = gen.Event{Name: InternalNatsServerReadyName, Node: ergoNodeName}
)

type InternalNatsServerReadyEvent struct {
	AgentCredentials []agentCredential
}
