package agentcommon

import "github.com/nats-io/nats.go"

// An agent client is used by a Nex node host to communicate with
// an agent.
type AgentClient struct {
	internalNatsConn *nats.Conn
	agentName        string
}

func NewAgentClient(nc *nats.Conn, name string) (*AgentClient, error) {
	return &AgentClient{internalNatsConn: nc, agentName: name}, nil
}
