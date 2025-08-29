package inmem

import (
	"context"
	"errors"
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

	agentNameDefault string = "inmem"
	agentTypeDefault string = "inmem"
	VERSION          string = "0.0.0"
)

type InMemAgent struct {
	Name         string
	WorkloadType string
	Nexus        string
	Version      string
	Workloads    Workloads
	XPair        nkeys.KeyPair
	StartTime    time.Time
	Runner       *agent.Runner

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

type InMemAgentOpt func(*InMemAgent) error

func WithAgentName(name string) InMemAgentOpt {
	return func(a *InMemAgent) error {
		a.Name = name
		return nil
	}
}

func WithWorkloadType(workloadType string) InMemAgentOpt {
	return func(a *InMemAgent) error {
		a.WorkloadType = workloadType
		return nil
	}
}

func NewInMemAgent(nexus, nodeId string, logger *slog.Logger, opts ...InMemAgentOpt) (*agent.Runner, error) {
	inmemAgent, err := newInMemAgent(nexus, nodeId, logger, opts...)

	runnerOpts := []agent.RunnerOpt{
		agent.WithLogger(logger),
	}

	if !nkeys.IsValidPublicServerKey(nodeId) {
		return nil, errors.New("node id is not a valid public server key")
	}

	inmemAgent.Runner, err = agent.NewRunner(context.Background(), nexus, nodeId, inmemAgent, runnerOpts...)
	if err != nil {
		return nil, err
	}

	return inmemAgent.Runner, nil
}

func newInMemAgent(nexus, nodeId string, logger *slog.Logger, opts ...InMemAgentOpt) (*InMemAgent, error) {
	xkp, err := nkeys.CreateCurveKeys()
	if err != nil {
		return nil, err
	}

	inmemAgent := &InMemAgent{
		Name:         agentNameDefault,
		WorkloadType: agentTypeDefault,
		Nexus:        nexus,
		Version:      VERSION,
		Workloads: Workloads{
			State: make(map[string][]InMemWorkload),
		},
		XPair:     xkp,
		StartTime: time.Now(),
		Logger:    logger,
	}

	for _, opt := range opts {
		if err := opt(inmemAgent); err != nil {
			return nil, err
		}
	}

	return inmemAgent, nil
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
		RegisterType:       a.WorkloadType,
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

	workloadCount := 0
	for _, workloads := range a.Workloads.State {
		workloadCount += len(workloads)
	}

	status := &models.AgentHeartbeat{
		Data: "In-Memory Nexlet",
		Summary: models.AgentSummary{
			Name:                a.Name,
			Type:                a.WorkloadType,
			StartTime:           a.StartTime,
			State:               "running",
			SupportedLifecycles: "service,job,function",
			WorkloadCount:       workloadCount,
			Version:             a.Version,
		},
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

	err := a.Runner.EmitEvent(startRequest.Request.Namespace, models.WorkloadStartedEvent{
		Id:           workloadId,
		Metadata:     models.WorkloadStartedEventMetadata{},
		Namespace:    startRequest.Request.Namespace,
		WorkloadType: a.WorkloadType,
	})
	if err != nil {
		a.Logger.Error("failed to emit workload started event", slog.String("workloadId", workloadId), slog.String("namespace", startRequest.Request.Namespace), slog.String("name", startRequest.Request.Name))
	}
	a.Logger.Debug("StartWorkload successful")

	if startRequest.Request.WorkloadLifecycle == models.WorkloadLifecycleFunction {
		err = a.Runner.RegisterTrigger(workloadId, startRequest.Request.Namespace, workloadId, &startRequest.WorkloadCreds, func(_ []byte) ([]byte, error) {
			a.Logger.Debug("Function trigger invoked", slog.String("workloadId", workloadId))
			return []byte("Function executed successfully"), nil
		})
		if err != nil {
			a.Logger.Error("failed to register function trigger", slog.String("workloadId", workloadId), slog.String("namespace", startRequest.Request.Namespace), slog.String("name", startRequest.Request.Name), slog.Any("error", err))
			return nil, err
		}
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

			err := a.Runner.EmitEvent(stopRequest.Namespace, models.WorkloadStoppedEvent{
				Id:           workloadId,
				Metadata:     models.WorkloadStoppedEventMetadata{},
				Namespace:    stopRequest.Namespace,
				WorkloadType: a.WorkloadType,
			})
			if err != nil {
				a.Logger.Error("failed to emit workload stopped event", slog.String("workloadId", workloadId), slog.String("namespace", stopRequest.Namespace))
			}
			a.Logger.Debug("StopWorkload successful", slog.String("workloadId", workloadId))

			if workload.startRequest.WorkloadLifecycle == models.WorkloadLifecycleFunction {
				err := a.Runner.UnregisterTrigger(workloadId)
				if err != nil {
					a.Logger.Error("failed to unregister function trigger", slog.String("workloadId", workloadId), slog.String("namespace", stopRequest.Namespace), slog.Any("error", err))
				} else {
					a.Logger.Debug("Function trigger unregistered successfully", slog.String("workloadId", workloadId))
				}
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
			WorkloadType:      a.WorkloadType,
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
		Type:                a.WorkloadType,
		StartTime:           a.StartTime,
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
