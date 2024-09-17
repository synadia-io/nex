package actors

import (
	"ergo.services/ergo/gen"
)

const (
	InternalNatsServerReadyName = gen.Atom("internal_nats_server_ready")
)

var (
	InternalNatsServerReady = gen.Event{Name: InternalNatsServerReadyName, Node: "nex@localhost"}
)

type InternalNatsServerReadyEvent struct {
	AgentCredentials []agentCredential
}
