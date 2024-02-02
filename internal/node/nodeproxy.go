package nexnode

import (
	"log/slog"

	"github.com/nats-io/nats-server/v2/server"
)

// Use this proxy object with extreme care, as it exposes
// the private/internal bits of a node instance to callers.
// It was created only as a way to make writing specs work
// and should not be used for any other purpose!
type NodeProxy struct {
	n *Node
}

func NewNodeProxyWith(node *Node) *NodeProxy {
	return &NodeProxy{n: node}
}

func (n *NodeProxy) APIListener() *ApiListener {
	return n.n.api
}

func (n *NodeProxy) MachineManager() *MachineManager {
	return n.n.manager
}

func (n *NodeProxy) Log() *slog.Logger {
	return n.n.log
}

func (n *NodeProxy) NodeConfiguration() *NodeConfiguration {
	return n.n.config
}

func (n *NodeProxy) InternalNATS() *server.Server {
	return n.n.natsint
}

func (n *NodeProxy) Telemetry() *Telemetry {
	return n.n.telemetry
}
