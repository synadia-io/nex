package nexnode

import (
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/models"
	internalnats "github.com/synadia-io/nex/internal/node/internal-nats"
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

func (n *NodeProxy) AgentManager() *AgentManager {
	return n.n.manager
}

func (n *NodeProxy) Log() *slog.Logger {
	return n.n.log
}

func (n *NodeProxy) NodeConfiguration() *models.NodeConfiguration {
	return n.n.config
}

func (n *NodeProxy) Telemetry() *observability.Telemetry {
	return n.n.telemetry
}

type AgentManagerProxy struct {
	m *AgentManager
}

func NewAgentManagerProxyWith(manager *AgentManager) *AgentManagerProxy {
	return &AgentManagerProxy{m: manager}
}

func (m *AgentManagerProxy) Log() *slog.Logger {
	return m.m.log
}

func (m *AgentManagerProxy) NodeConfiguration() *models.NodeConfiguration {
	return m.m.config
}

func (m *AgentManagerProxy) InternalNATS() *internalnats.InternalNatsServer {
	return m.m.natsint
}

func (m *AgentManagerProxy) InternalNATSConn() *nats.Conn {
	return m.m.ncint
}

func (m *AgentManagerProxy) Telemetry() *observability.Telemetry {
	return m.m.t
}

// func (w *AgentManagerProxy) AllAgents() map[string]*processmanager.ProcessInfo {
// 	agentsmap := make(map[string]*processmanager.ProcessInfo)

// 	for _, agent := range w.Agents() {
// 		agentsmap[agent.ID] = agent
// 	}

// 	for _, agent := range w.PoolAgents() {
// 		agentsmap[agent.ID] = agent
// 	}

// 	return agentsmap
// }

// func (w *AgentManagerProxy) Agents() map[string]*processmanager.ProcessInfo {
// 	agentsmap := make(map[string]*processmanager.ProcessInfo)

// 	agents, _ := w.m.procMan.ListProcesses()
// 	for _, agent := range agents {
// 		agentsmap[agent.ID] = &agent
// 	}

// 	return agentsmap
// }

func (w *AgentManagerProxy) PoolAgents() map[string]*processmanager.ProcessInfo {
	// FIXME-- this is no longer exposed
	return map[string]*processmanager.ProcessInfo{}
}

type AgentProxy struct {
	agent *processmanager.ProcessInfo
}

func NewAgentProxyWith(agent *processmanager.ProcessInfo) *AgentProxy {
	return &AgentProxy{agent: agent}
}
