package actors

import (
	"ergo.services/ergo/gen"
)

const (
	InternalNatsServerReadyName = gen.Atom("internal_nats_server_ready")
	PostInit                    = "post_init"
)

var (
	InternalNatsServerReady = gen.Event{Name: InternalNatsServerReadyName, Node: "nex@localhost"}
)

type InternalNatsServerReadyEvent struct {
	AgentCredentials []agentCredential
}