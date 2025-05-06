package inmem

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/synadia-io/nexlet.go/agent"
	"github.com/synadia-labs/nex/models"

	"github.com/nats-io/nkeys"
)

var (
	_ agent.Agent = (*inMemAgent)(nil)

	agentName string = "inmem"
	VERSION   string = "0.0.0"
)

type inMemAgent struct {
	name      string
	version   string
	workloads workloads
	xPair     nkeys.KeyPair
	startTime time.Time
	runner    *agent.Runner
}

type inMemWorkload struct {
	id   string
	name string

	startTime    time.Time
	startRequest *models.StartWorkloadRequest
}

type workloads struct {
	sync.RWMutex
	state map[string][]inMemWorkload
}

func NewInMemAgent(nodeId string, logger *slog.Logger) (*agent.Runner, error) {
	xkp, err := nkeys.CreateCurveKeys()
	if err != nil {
		return nil, err
	}

	inmemAgent := &inMemAgent{
		name:    agentName,
		version: VERSION,
		workloads: workloads{
			state: make(map[string][]inMemWorkload),
		},
		xPair:     xkp,
		startTime: time.Now(),
	}

	opts := []agent.RunnerOpt{
		agent.WithLogger(logger),
	}

	if !nkeys.IsValidPublicServerKey(nodeId) {
		return nil, errors.New("node id is not a valid public server key")
	}

	inmemAgent.runner, err = agent.NewRunner(context.Background(), nodeId, inmemAgent, opts...)
	if err != nil {
		return nil, err
	}

	return inmemAgent.runner, nil
}

func (a *inMemAgent) Register() (*models.RegisterAgentRequest, error) {
	pub, err := a.xPair.PublicKey()
	if err != nil {
		return nil, err
	}
	return &models.RegisterAgentRequest{
		Description:        "In memory no-op agent",
		MaxWorkloads:       0,
		Name:               a.name,
		RegisterType:       a.name,
		PublicXkey:         pub,
		StartRequestSchema: "{}",
		SupportedLifecycles: []models.WorkloadLifecycle{
			models.WorkloadLifecycleService,
			models.WorkloadLifecycleJob,
			models.WorkloadLifecycleFunction,
		},
		Version: a.version,
	}, nil
}

func (a *inMemAgent) Heartbeat() (*models.AgentHeartbeat, error) {
	status := &models.AgentHeartbeat{
		Data:          "In-Memory Nexlet",
		State:         "running",
		WorkloadCount: len(a.workloads.state),
	}

	return status, nil
}

func (a *inMemAgent) StartWorkload(workloadId string, startRequest *models.AgentStartWorkloadRequest, existing bool) (*models.StartWorkloadResponse, error) {
	if existing {
		slog.Info("restarting existing workload", slog.String("workloadId", workloadId))
	}

	a.workloads.Lock()
	defer a.workloads.Unlock()

	if a.workloads.state[startRequest.Request.Namespace] == nil {
		a.workloads.state[startRequest.Request.Namespace] = []inMemWorkload{}
	}

	if startRequest.Request.Name == "" {
		startRequest.Request.Name = workloadId
	}

	a.workloads.state[startRequest.Request.Namespace] = append(a.workloads.state[startRequest.Request.Namespace], inMemWorkload{
		name:         startRequest.Request.Name,
		id:           workloadId,
		startTime:    time.Now(),
		startRequest: &startRequest.Request,
	})

	return &models.StartWorkloadResponse{
		Id:   workloadId,
		Name: startRequest.Request.Name,
	}, nil
}

func (a *inMemAgent) StopWorkload(workloadId string, stopRequest *models.StopWorkloadRequest) error {
	a.workloads.Lock()
	defer a.workloads.Unlock()

	workloads, ok := a.workloads.state[stopRequest.Namespace]
	if !ok {
		fmt.Printf("no workloads found for namespace %s", stopRequest.Namespace)
		return errors.New("workload not found")
	}

	for i, workload := range workloads {
		if workload.id == workloadId {
			workloads = append(workloads[:i], workloads[i+1:]...)
			a.workloads.state[stopRequest.Namespace] = workloads
			return nil
		}
	}

	return errors.New("workload not found")
}

func (a *inMemAgent) QueryWorkloads(namespace string, filter []string) (*models.AgentListWorkloadsResponse, error) {
	a.workloads.RLock()
	defer a.workloads.RUnlock()

	workloads, ok := a.workloads.state[namespace]
	if !ok {
		return nil, errors.New("namespace not found")
	}

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

	if len(filter) > 0 {
		filteredResp := models.AgentListWorkloadsResponse{}
		for _, workload := range resp {
			if slices.Contains(filter, workload.Id) {
				filteredResp = append(filteredResp, workload)
			}
		}
		resp = filteredResp
	}

	return &resp, nil
}

func (a *inMemAgent) SetLameduck(before time.Duration) error {
	a.workloads.Lock()
	defer a.workloads.Unlock()

	for k := range a.workloads.state {
		delete(a.workloads.state, k)
	}
	return nil
}

func (a *inMemAgent) Ping() (*models.AgentSummary, error) {
	a.workloads.RLock()
	defer a.workloads.RUnlock()

	workloadCount := 0
	for _, workloads := range a.workloads.state {
		workloadCount += len(workloads)
	}
	return &models.AgentSummary{
		Name:                a.name,
		StartTime:           a.startTime.String(),
		State:               "running",
		SupportedLifecycles: "service,job,function",
		WorkloadCount:       workloadCount,
		Version:             a.version,
	}, nil
}

func (a *inMemAgent) PingWorkload(inWorkloadId string) bool {
	a.workloads.RLock()
	defer a.workloads.RUnlock()

	for _, workloads := range a.workloads.state {
		for _, workload := range workloads {
			if workload.id == inWorkloadId {
				return true
			}
		}
	}
	return false
}

func (a *inMemAgent) GetWorkload(workloadId, targetXkey string) (*models.StartWorkloadRequest, error) {
	a.workloads.RLock()
	defer a.workloads.RUnlock()

	for _, workloads := range a.workloads.state {
		for _, workload := range workloads {
			if workload.id == workloadId {
				return workload.startRequest, nil
			}
		}
	}
	return nil, errors.New("workload not found")
}
