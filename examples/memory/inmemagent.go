package inmem

import (
	"errors"
	"time"

	"github.com/synadia-io/nexlet.go/agent"
	"github.com/synadia-labs/nex/models"

	"github.com/nats-io/nkeys"
)

var _ agent.Agent = (*InMemAgent)(nil)

type InMemAgent struct {
	Name      string
	Version   string
	Workloads map[string][]inMemWorkload // map[namespace]workloads
	XPair     nkeys.KeyPair
	StartTime time.Time
}

type inMemWorkload struct {
	id   string
	name string

	startTime    time.Time
	startRequest *models.StartWorkloadRequest
}

func (a *InMemAgent) Register(agentId string) (*models.RegisterAgentRequest, error) {
	pub, err := a.XPair.PublicKey()
	if err != nil {
		return nil, err
	}
	return &models.RegisterAgentRequest{
		AssignedId:          agentId,
		Description:         "In memory no-op agent",
		MaxWorkloads:        0,
		Name:                "inmem",
		PublicXkey:          pub,
		StartRequestSchema:  "{}",
		SupportedLifecycles: []models.WorkloadLifecycle{models.WorkloadLifecycleService},
		Version:             "0.0.0",
	}, nil
}

func (a *InMemAgent) Heartbeat() (*models.AgentHeartbeat, error) {
	status := &models.AgentHeartbeat{
		Data:          "Some random data",
		State:         "running",
		WorkloadCount: len(a.Workloads),
	}

	return status, nil
}

func (a *InMemAgent) StartWorkload(workloadId string, startRequest *models.StartWorkloadRequest) (*models.StartWorkloadResponse, error) {
	if a.Workloads[startRequest.Namespace] == nil {
		a.Workloads[startRequest.Namespace] = []inMemWorkload{}
	}

	if startRequest.Name == "" {
		startRequest.Name = workloadId
	}

	a.Workloads[startRequest.Namespace] = append(a.Workloads[startRequest.Namespace], inMemWorkload{
		id:           workloadId,
		startTime:    time.Now(),
		startRequest: startRequest,
	})

	return &models.StartWorkloadResponse{
		Id:   workloadId,
		Name: startRequest.Name,
	}, nil
}

func (a *InMemAgent) StopWorkload(workloadId string, stopRequest *models.StopWorkloadRequest) error {
	workloads, ok := a.Workloads[stopRequest.Namespace]
	if !ok {
		return errors.New("namespace not found")
	}

	for i, workload := range workloads {
		if workload.id == workloadId {
			workloads = append(workloads[:i], workloads[i+1:]...)
			a.Workloads[stopRequest.Namespace] = workloads
			return nil
		}
	}

	return errors.New("workload not found")
}

func (a *InMemAgent) QueryWorkloads(namespace string) (*models.AgentListWorkloadsResponse, error) {
	workloads, ok := a.Workloads[namespace]
	if !ok {
		return nil, errors.New("namespace not found")
	}

	resp := models.AgentListWorkloadsResponse{}
	for _, workload := range workloads {
		resp = append(resp, models.WorkloadSummary{
			Id:              workload.id,
			Name:            workload.name,
			Runtime:         time.Since(workload.startTime).String(),
			StartTime:       workload.startTime.Format(time.RFC3339),
			WorkloadRuntype: "inmem",
			WorkloadState:   models.WorkloadStateRunning,
			WorkloadType:    "service",
		})
	}

	return &resp, nil
}

func (a *InMemAgent) SetLameduck(before time.Duration) error {
	for k := range a.Workloads {
		delete(a.Workloads, k)
	}
	return nil
}

func (a *InMemAgent) Ping() (*models.AgentSummary, error) {
	return &models.AgentSummary{
		Name:                a.Name,
		StartTime:           a.StartTime.String(),
		State:               "running",
		SupportedLifecycles: "service",
		WorkloadCount:       0,
		Version:             a.Version,
	}, nil
}
