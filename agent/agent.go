// Package agent defines the interface for an agent that interacts with a workload management system.
package agent

import (
	"time"

	"github.com/synadia-labs/nex/models"
)

type Agent interface {
	Register() (*models.RegisterAgentRequest, error)
	Heartbeat() (*models.AgentHeartbeat, error)
	StartWorkload(workloadID string, req *models.AgentStartWorkloadRequest, existing bool) (*models.StartWorkloadResponse, error)
	StopWorkload(workloadID string, req *models.StopWorkloadRequest) error
	GetWorkload(workloadID, targetXkey string) (*models.StartWorkloadRequest, error)
	QueryWorkloads(namespace string, filter []string) (*models.AgentListWorkloadsResponse, error)
	SetLameduck(before time.Duration) error
	Ping() (*models.AgentSummary, error)
}

// AgentIngessWorkloads Optional interface for agents that support ingressable workloads
type AgentIngessWorkloads interface {
	PingWorkload(workloadID string) bool
	GetWorkloadExposedPorts(workloadID string) ([]int, error)
}

type AgentEventListener interface {
	EventListener(msg []byte)
}
