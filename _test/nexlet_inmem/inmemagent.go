package inmem

import (
	"errors"
	"log/slog"
	"slices"
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
	workloads map[string][]inMemWorkload // map[namespace]workloads
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

func NewInMemAgent(nodeId string, logger *slog.Logger) (*agent.Runner, error) {
	xkp, err := nkeys.CreateCurveKeys()
	if err != nil {
		return nil, err
	}

	inmemAgent := &inMemAgent{
		name:      agentName,
		version:   VERSION,
		workloads: make(map[string][]inMemWorkload),
		xPair:     xkp,
		startTime: time.Now(),
	}

	opts := []agent.RunnerOpt{
		agent.WithLogger(logger),
	}

	if nkeys.IsValidPublicServerKey(nodeId) {
		opts = append(opts, agent.AsLocalAgent(nodeId))
	} else {
		return nil, errors.New("node id is not a valid public server key")
	}

	inmemAgent.runner, err = agent.NewRunner(agentName, VERSION, inmemAgent, opts...)
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
		WorkloadCount: len(a.workloads),
	}

	return status, nil
}

func (a *inMemAgent) StartWorkload(workloadId string, startRequest *models.AgentStartWorkloadRequest, existing bool) (*models.StartWorkloadResponse, error) {
	if existing {
		slog.Info("restarting existing workload", slog.String("workloadId", workloadId))
	}

	if a.workloads[startRequest.Request.Namespace] == nil {
		a.workloads[startRequest.Request.Namespace] = []inMemWorkload{}
	}

	if startRequest.Request.Name == "" {
		startRequest.Request.Name = workloadId
	}

	a.workloads[startRequest.Request.Namespace] = append(a.workloads[startRequest.Request.Namespace], inMemWorkload{
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
	workloads, ok := a.workloads[stopRequest.Namespace]
	if !ok {
		return errors.New("namespace not found")
	}

	for i, workload := range workloads {
		if workload.id == workloadId {
			workloads = append(workloads[:i], workloads[i+1:]...)
			a.workloads[stopRequest.Namespace] = workloads
			return nil
		}
	}

	return errors.New("workload not found")
}

func (a *inMemAgent) QueryWorkloads(namespace string, filter []string) (*models.AgentListWorkloadsResponse, error) {
	workloads, ok := a.workloads[namespace]
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
	for k := range a.workloads {
		delete(a.workloads, k)
	}
	return nil
}

func (a *inMemAgent) Ping() (*models.AgentSummary, error) {
	workloadCount := 0
	for _, workloads := range a.workloads {
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
	for _, workloads := range a.workloads {
		for _, workload := range workloads {
			if workload.id == inWorkloadId {
				return true
			}
		}
	}
	return false
}

func (a *inMemAgent) GetWorkload(workloadId, targetXkey string) (*models.StartWorkloadRequest, error) {
	for _, workloads := range a.workloads {
		for _, workload := range workloads {
			if workload.id == workloadId {
				return workload.startRequest, nil
			}
		}
	}
	return nil, errors.New("workload not found")
}
