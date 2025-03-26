package agent

import (
	"time"

	"github.com/synadia-labs/nex/models"
)

type Agent interface {
	Register() (*models.RegisterAgentRequest, error)
	Heartbeat() (*models.AgentHeartbeat, error)
	StartWorkload(workloadId string, req *models.AgentStartWorkloadRequest, existing bool) (*models.StartWorkloadResponse, error)
	StopWorkload(workloadId string, req *models.StopWorkloadRequest) error
	GetWorkload(workloadId, targetXkey string) (*models.StartWorkloadRequest, error)
	QueryWorkloads(namespace string, filter []string) (*models.AgentListWorkloadsResponse, error)
	SetLameduck(before time.Duration) error
	Ping() (*models.AgentSummary, error)
}

// Optional interface for agents that support ingressable workloads
type AgentIngessWorkloads interface {
	GetIngressData() (*models.AgentIngressData, error)
	PingWorkload(workloadId string) bool
	GetWorkloadExposedPorts(workloadId string) ([]int, error)
}

type AgentEventListener interface {
	EventListener(msg []byte)
}
