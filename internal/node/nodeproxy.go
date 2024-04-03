package nexnode

import (
	"log/slog"
	"net"

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

func (n *NodeProxy) WorkloadManager() *WorkloadManager {
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

type WorkloadManagerProxy struct {
	m *WorkloadManager
}

func NewWorkloadManagerProxyWith(manager *WorkloadManager) *WorkloadManagerProxy {
	return &WorkloadManagerProxy{m: manager}
}

func (m *WorkloadManagerProxy) Log() *slog.Logger {
	return m.m.log
}

func (m *WorkloadManagerProxy) NodeConfiguration() *NodeConfiguration {
	return m.m.config
}

func (m *WorkloadManagerProxy) InternalNATSConn() *nats.Conn {
	return m.m.ncInternal
}

func (m *WorkloadManagerProxy) Telemetry() *Telemetry {
	return m.m.t
}

func (w *WorkloadManagerProxy) AllAgents() map[string]*AgentInfo {
	agentsmap := make(map[string]*AgentInfo)

	for _, agent := range w.Agents() {
		agentsmap[agent.ID] = agent
	}

	for _, agent := range w.PoolAgents() {
		agentsmap[agent.ID] = agent
	}

	return agentsmap
}

func (w *WorkloadManagerProxy) Agents() map[string]*AgentInfo {
	agentsmap := make(map[string]*AgentInfo)

	agents, _ := w.m.procMan.ListProcesses()
	for _, agent := range agents {
		agentsmap[agent.ID] = &agent
	}

	return agentsmap
}

func (w *WorkloadManagerProxy) PoolAgents() map[string]*AgentInfo {
	agentsmap := make(map[string]*AgentInfo)

	agents, _ := w.m.procMan.ListPool()
	for _, agent := range agents {
		agentsmap[agent.ID] = &agent
	}

	return agentsmap
}

type AgentProxy struct {
	agent *AgentInfo
}

func NewAgentProxyWith(agent *AgentInfo) *AgentProxy {
	return &AgentProxy{agent: agent}
}

func (a *AgentProxy) IP() net.IP {
	return *a.agent.IP
}
