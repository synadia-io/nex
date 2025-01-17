package agent

import (
	"time"

	"github.com/synadia-labs/nex/models"
)

type Agent interface {
	Register(agentId string) (*models.RegisterAgentRequest, error)
	Heartbeat() (*models.AgentHeartbeat, error)
	StartWorkload(workloadId string, req *models.StartWorkloadRequest) (*models.StartWorkloadResponse, error)
	StopWorkload(workloadId string, req *models.StopWorkloadRequest) error
	QueryWorkloads(namespace string) (*models.AgentListWorkloadsResponse, error)
	SetLameduck(before time.Duration) error
	Ping() (*models.AgentSummary, error)
}
