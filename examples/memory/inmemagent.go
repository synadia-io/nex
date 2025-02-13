package inmem

import (
	"errors"
	"log/slog"
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

func (a *InMemAgent) Register() (*models.RegisterAgentRequest, error) {
	pub, err := a.XPair.PublicKey()
	if err != nil {
		return nil, err
	}
	return &models.RegisterAgentRequest{
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

func (a *InMemAgent) StartWorkload(workloadId string, startRequest *models.AgentStartWorkloadRequest, existing bool) (*models.StartWorkloadResponse, error) {
	if existing {
		slog.Info("restarting existing workload", slog.String("workloadId", workloadId))
	}

	if a.Workloads[startRequest.Request.Namespace] == nil {
		a.Workloads[startRequest.Request.Namespace] = []inMemWorkload{}
	}

	if startRequest.Request.Name == "" {
		startRequest.Request.Name = workloadId
	}

	a.Workloads[startRequest.Request.Namespace] = append(a.Workloads[startRequest.Request.Namespace], inMemWorkload{
		id:           workloadId,
		startTime:    time.Now(),
		startRequest: &startRequest.Request,
	})

	return &models.StartWorkloadResponse{
		Id:   workloadId,
		Name: startRequest.Request.Name,
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

func (a *InMemAgent) QueryWorkloads(namespace string, filter []string) (*models.AgentListWorkloadsResponse, error) {
	workloads, ok := a.Workloads[namespace]
	if !ok {
		return nil, errors.New("namespace not found")
	}

	// no filter implemented in this example agent

	resp := models.AgentListWorkloadsResponse{}
	for _, workload := range workloads {
		resp = append(resp, models.WorkloadSummary{
			Id:                workload.id,
			Name:              workload.name,
			Runtime:           time.Since(workload.startTime).String(),
			StartTime:         workload.startTime.Format(time.RFC3339),
			WorkloadType:      "inmem",
			WorkloadState:     models.WorkloadStateRunning,
			WorkloadLifecycle: "service",
			Metadata:          map[string]string{"extra": "metadata"},
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

func (a *InMemAgent) PingWorkload(id string) bool {
	for _, namespace := range a.Workloads {
		for _, workload := range namespace {
			if workload.id == id {
				return true
			}
		}
	}
	return false
}

func (a *InMemAgent) GetWorkload(workloadId, targetXkey string) (*models.StartWorkloadRequest, error) {
	// TODO: implement encryption for target
	for _, workloads := range a.Workloads {
		for _, workload := range workloads {
			if workload.id == workloadId {
				return workload.startRequest, nil
			}
		}
	}
	return nil, errors.New("workload not found")
}
