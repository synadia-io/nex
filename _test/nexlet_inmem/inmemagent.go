package inmem

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/synadia-io/nexlet.go/agent"
	"github.com/synadia-labs/nex/models"

	"github.com/nats-io/nkeys"
)

var (
	_ agent.Agent = (*InMemAgent)(nil)

	agentName string = "inmem"
	VERSION   string = "0.0.0"
)

type InMemAgent struct {
	Name      string
	Version   string
	Workloads Workloads
	XPair     nkeys.KeyPair
	StartTime time.Time
	Runner    *agent.Runner

	Logger *slog.Logger
}

type InMemWorkload struct {
	id   string
	name string

	startTime    time.Time
	startRequest *models.StartWorkloadRequest
}

type Workloads struct {
	sync.RWMutex
	State map[string][]InMemWorkload
}

func NewInMemAgent(nodeId string, logger *slog.Logger) (*agent.Runner, error) {
	xkp, err := nkeys.CreateCurveKeys()
	if err != nil {
		return nil, err
	}

	inmemAgent := &InMemAgent{
		Name:    agentName,
		Version: VERSION,
		Workloads: Workloads{
			State: make(map[string][]InMemWorkload),
		},
		XPair:     xkp,
		StartTime: time.Now(),
		Logger:    logger,
	}

	opts := []agent.RunnerOpt{
		agent.WithLogger(logger),
	}

	if !nkeys.IsValidPublicServerKey(nodeId) {
		return nil, errors.New("node id is not a valid public server key")
	}

	inmemAgent.Runner, err = agent.NewRunner(context.Background(), nodeId, inmemAgent, opts...)
	if err != nil {
		return nil, err
	}

	return inmemAgent.Runner, nil
}

func (a *InMemAgent) Register() (*models.RegisterAgentRequest, error) {
	pub, err := a.XPair.PublicKey()
	if err != nil {
		return nil, err
	}
	return &models.RegisterAgentRequest{
		Description:        "In memory no-op agent",
		MaxWorkloads:       0,
		Name:               a.Name,
		RegisterType:       a.Name,
		PublicXkey:         pub,
		StartRequestSchema: "{}",
		SupportedLifecycles: []models.WorkloadLifecycle{
			models.WorkloadLifecycleService,
			models.WorkloadLifecycleJob,
			models.WorkloadLifecycleFunction,
		},
		Version: a.Version,
	}, nil
}

func (a *InMemAgent) Heartbeat() (*models.AgentHeartbeat, error) {
	a.Workloads.Lock()
	defer a.Workloads.Unlock()

	status := &models.AgentHeartbeat{
		Data:          "In-Memory Nexlet",
		State:         "running",
		WorkloadCount: len(a.Workloads.State),
	}

	return status, nil
}

func (a *InMemAgent) StartWorkload(workloadId string, startRequest *models.AgentStartWorkloadRequest, existing bool) (*models.StartWorkloadResponse, error) {
	a.Logger.Debug("StartWorkload received", slog.String("workloadId", workloadId), slog.String("namespace", startRequest.Request.Namespace), slog.String("name", startRequest.Request.Name))

	if existing {
		a.Logger.Info("restarting existing workload", slog.String("workloadId", workloadId))
	}

	a.Workloads.Lock()
	defer a.Workloads.Unlock()

	if a.Workloads.State[startRequest.Request.Namespace] == nil {
		a.Workloads.State[startRequest.Request.Namespace] = []InMemWorkload{}
	}

	if startRequest.Request.Name == "" {
		startRequest.Request.Name = workloadId
	}

	a.Workloads.State[startRequest.Request.Namespace] = append(a.Workloads.State[startRequest.Request.Namespace], InMemWorkload{
		name:         startRequest.Request.Name,
		id:           workloadId,
		startTime:    time.Now(),
		startRequest: &startRequest.Request,
	})

	a.Logger.Debug("StartWorkload successful")
	err := a.Runner.EmitEvent(models.WorkloadStartedEvent{
		Id:        workloadId,
		Metadata:  models.WorkloadStartedEventMetadata{},
		Namespace: startRequest.Request.Namespace,
	})
	if err != nil {
		a.Logger.Error("failed to emit workload started event", slog.String("workloadId", workloadId), slog.String("namespace", startRequest.Request.Namespace), slog.String("name", startRequest.Request.Name))
	}
	return &models.StartWorkloadResponse{
		Id:   workloadId,
		Name: startRequest.Request.Name,
	}, nil
}

func (a *InMemAgent) StopWorkload(workloadId string, stopRequest *models.StopWorkloadRequest) error {
	a.Logger.Debug("StopWorkload received", slog.String("workloadId", workloadId), slog.String("namespace", stopRequest.Namespace))

	a.Workloads.Lock()
	defer a.Workloads.Unlock()

	workloads, ok := a.Workloads.State[stopRequest.Namespace]
	if !ok {
		fmt.Printf("no workloads found for namespace %s", stopRequest.Namespace)
		return errors.New(string(models.GenericErrorsNamespaceNotFound))
	}

	for i, workload := range workloads {
		if workload.id == workloadId {
			workloads = slices.Delete(workloads, i, i+1)
			if len(workloads) == 0 {
				delete(a.Workloads.State, stopRequest.Namespace)
			} else {
				a.Workloads.State[stopRequest.Namespace] = workloads
			}
			a.Logger.Debug("StopWorkload successful", slog.String("workloadId", workloadId))
			err := a.Runner.EmitEvent(models.WorkloadStoppedEvent{
				Id:        workloadId,
				Metadata:  models.WorkloadStoppedEventMetadata{},
				Namespace: stopRequest.Namespace,
			})
			if err != nil {
				a.Logger.Error("failed to emit workload stopped event", slog.String("workloadId", workloadId), slog.String("namespace", stopRequest.Namespace))
			}
			return nil
		}
	}

	return errors.New(string(models.GenericErrorsWorkloadNotFound))
}

func (a *InMemAgent) QueryWorkloads(namespace string, filter []string) (*models.AgentListWorkloadsResponse, error) {
	a.Logger.Debug("QueryWorkloads received", slog.String("namespace", namespace), slog.String("filter", strings.Join(filter, ",")))
	a.Workloads.RLock()
	defer a.Workloads.RUnlock()

	workloads, ok := a.Workloads.State[namespace]
	if !ok {
		a.Logger.Debug("namespace not found", slog.String("namespace", namespace))
		return &models.AgentListWorkloadsResponse{}, nil
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

	a.Logger.Debug("QueryWorkloads successful", slog.String("namespace", namespace), slog.String("filter", strings.Join(filter, ",")))
	return &resp, nil
}

func (a *InMemAgent) SetLameduck(before time.Duration) error {
	a.Logger.Debug("SetLameduck received", slog.Duration("before", before))

	a.Workloads.Lock()
	defer a.Workloads.Unlock()

	for k := range a.Workloads.State {
		delete(a.Workloads.State, k)
	}

	a.Logger.Debug("SetLameduck successful")
	return nil
}

func (a *InMemAgent) Ping() (*models.AgentSummary, error) {
	a.Workloads.RLock()
	defer a.Workloads.RUnlock()

	workloadCount := 0
	for _, workloads := range a.Workloads.State {
		workloadCount += len(workloads)
	}
	return &models.AgentSummary{
		Name:                a.Name,
		StartTime:           a.StartTime.String(),
		State:               "running",
		SupportedLifecycles: "service,job,function",
		WorkloadCount:       workloadCount,
		Version:             a.Version,
	}, nil
}

func (a *InMemAgent) PingWorkload(inWorkloadId string) bool {
	a.Workloads.RLock()
	defer a.Workloads.RUnlock()

	for _, workloads := range a.Workloads.State {
		for _, workload := range workloads {
			if workload.id == inWorkloadId {
				return true
			}
		}
	}
	return false
}

func (a *InMemAgent) GetWorkload(workloadId, targetXkey string) (*models.StartWorkloadRequest, error) {
	a.Workloads.RLock()
	defer a.Workloads.RUnlock()

	for _, workloads := range a.Workloads.State {
		for _, workload := range workloads {
			if workload.id == workloadId {
				return workload.startRequest, nil
			}
		}
	}
	return nil, errors.New(string(models.GenericErrorsWorkloadNotFound))
}
