package actors

import (
	"ergo.services/ergo/gen"
)

const (
	InternalNatsServerReady = gen.Atom("internal_nats_server_ready")
)

var (
	InternalNatsServerReadyName = gen.Event{Name: InternalNatsServerReady, Node: "nex@localhost"}
)

type InternalNatsServerReadyEvent struct {
	AgentCredentials []agentCredential
}
