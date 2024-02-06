package nexnode

import (
	"log/slog"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
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

func (n *NodeProxy) InternalNATSConn() *nats.Conn {
	return n.n.ncint
}

func (n *NodeProxy) Telemetry() *Telemetry {
	return n.n.telemetry
}

type MachineManagerProxy struct {
	m *MachineManager
}

func NewMachineManagerProxyWith(manager *MachineManager) *MachineManagerProxy {
	return &MachineManagerProxy{m: manager}
}

func (m *MachineManagerProxy) Log() *slog.Logger {
	return m.m.log
}

func (m *MachineManagerProxy) NodeConfiguration() *NodeConfiguration {
	return m.m.config
}

func (m *MachineManagerProxy) InternalNATSConn() *nats.Conn {
	return m.m.ncInternal
}

func (m *MachineManagerProxy) Telemetry() *Telemetry {
	return m.m.t
}

func (m *MachineManagerProxy) VMs() map[string]*runningFirecracker {
	return m.m.allVMs
}

func (m *MachineManagerProxy) PoolVMs() chan *runningFirecracker {
	return m.m.warmVMs
}

