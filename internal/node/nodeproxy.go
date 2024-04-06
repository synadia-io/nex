package nexnode

import (
	"log/slog"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/models"
	"github.com/synadia-io/nex/internal/node/observability"
	"github.com/synadia-io/nex/internal/node/processmanager"
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

func (n *NodeProxy) NodeConfiguration() *models.NodeConfiguration {
	return n.n.config
}

func (n *NodeProxy) InternalNATS() *server.Server {
	return n.n.natsint
}

func (n *NodeProxy) InternalNATSConn() *nats.Conn {
	return n.n.ncint
}

func (n *NodeProxy) Telemetry() *observability.Telemetry {
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

func (m *WorkloadManagerProxy) NodeConfiguration() *models.NodeConfiguration {
	return m.m.config
}

func (m *WorkloadManagerProxy) InternalNATSConn() *nats.Conn {
	return m.m.ncInternal
}

func (m *WorkloadManagerProxy) Telemetry() *observability.Telemetry {
	return m.m.t
}

func (w *WorkloadManagerProxy) AllAgents() map[string]*processmanager.ProcessInfo {
	agentsmap := make(map[string]*processmanager.ProcessInfo)

	for _, agent := range w.Agents() {
		agentsmap[agent.ID] = agent
	}

	for _, agent := range w.PoolAgents() {
		agentsmap[agent.ID] = agent
	}

	return agentsmap
}

func (w *WorkloadManagerProxy) Agents() map[string]*processmanager.ProcessInfo {
	agentsmap := make(map[string]*processmanager.ProcessInfo)

	agents, _ := w.m.procMan.ListProcesses()
	for _, agent := range agents {
		agentsmap[agent.ID] = &agent
	}

	return agentsmap
}

func (w *WorkloadManagerProxy) PoolAgents() map[string]*processmanager.ProcessInfo {
	// FIXME-- this is no longer exposed
	return map[string]*processmanager.ProcessInfo{}
}

type AgentProxy struct {
	agent *processmanager.ProcessInfo
}

func NewAgentProxyWith(agent *processmanager.ProcessInfo) *AgentProxy {
	return &AgentProxy{agent: agent}
}
