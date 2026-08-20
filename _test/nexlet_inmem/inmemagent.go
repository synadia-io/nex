package inmem

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/nex/sdk/go/agent"

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

	// FailStops, when true, makes StopWorkload return an error without
	// touching agent-side workload state. Tests use it to simulate a nexlet
	// that cannot confirm a stop (e.g. crashed or unreachable).
	FailStops bool

	// StopDelay makes StopWorkload sleep before doing its work, simulating a
	// nexlet whose stop is synchronous and slow -- the native nexlet's is
	// bounded at ~5.75s (grace + SIGKILL + confirm). Tests use it to prove
	// the node's stop-confirmation wait outlasts a legitimate slow stop.
	StopDelay time.Duration

	// StartRequestSchema is the JSON schema this agent advertises at
	// registration. The node compiles it and validates every run_request
	// against it, on both the deploy and the update path. It defaults to
	// "{}" -- accepts anything -- which is what most tests want. A test
	// that needs the node's schema validation to actually REJECT a
	// run_request must register a restrictive schema (see
	// WithStartRequestSchema), otherwise there is nothing to fail on.
	StartRequestSchema string

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

// WithStartRequestSchema overrides the permissive default ("{}") run_request
// schema this agent registers with the node.
func WithStartRequestSchema(schema string) InMemAgentOpt {
	return func(a *InMemAgent) error {
		a.StartRequestSchema = schema
		return nil
	}
}

func NewInMemAgent(nexus, nodeId string, logger *slog.Logger, opts ...InMemAgentOpt) (*agent.Runner, error) {
	runner, _, err := NewInMemAgentWithHandle(nexus, nodeId, logger, opts...)
	return runner, err
}

// NewInMemAgentWithHandle behaves like NewInMemAgent but also returns the
// concrete *InMemAgent alongside its *agent.Runner. The runner alone does
// not expose the underlying agent, so tests that need to reach in-process
// test knobs after wiring (e.g. FailStops) must use this constructor.
func NewInMemAgentWithHandle(nexus, nodeId string, logger *slog.Logger, opts ...InMemAgentOpt) (*agent.Runner, *InMemAgent, error) {
	inmemAgent, err := newInMemAgent(nexus, nodeId, logger, opts...)
	if err != nil {
		return nil, nil, err
	}

	runnerOpts := []agent.RunnerOpt{
		agent.WithLogger(logger),
	}

	if !nkeys.IsValidPublicServerKey(nodeId) {
		return nil, nil, errors.New("node id is not a valid public server key")
	}

	inmemAgent.Runner, err = agent.NewRunner(context.Background(), nexus, nodeId, inmemAgent, runnerOpts...)
	if err != nil {
		return nil, nil, err
	}

	return inmemAgent.Runner, inmemAgent, nil
}

func newInMemAgent(nexus, nodeId string, logger *slog.Logger, opts ...InMemAgentOpt) (*InMemAgent, error) {
	xkp, err := nkeys.CreateCurveKeys()
	if err != nil {
		return nil, err
	}

	inmemAgent := &InMemAgent{
		Name:               agentNameDefault,
		WorkloadType:       agentTypeDefault,
		Nexus:              nexus,
		Version:            VERSION,
		StartRequestSchema: "{}",
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

	// Several tests build an InMemAgent as a plain struct literal rather
	// than through newInMemAgent, so the zero value has to keep meaning the
	// permissive schema: the node cannot compile "" and would reject the
	// registration outright.
	schema := a.StartRequestSchema
	if schema == "" {
		schema = "{}"
	}

	return &models.RegisterAgentRequest{
		Description:        "In memory no-op agent",
		MaxWorkloads:       0,
		Name:               a.Name,
		RegisterType:       a.WorkloadType,
		PublicXkey:         pub,
		StartRequestSchema: schema,
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

	newEntry := InMemWorkload{
		name:         startRequest.Request.Name,
		id:           workloadId,
		startTime:    time.Now(),
		startRequest: &startRequest.Request,
	}

	// Replace-by-id: a workload id already present in this namespace's
	// slice is overwritten in place rather than appended a second time.
	// UPDATE/RESTART's node-side composition always stops the old instance
	// first (handlers.go replaceWorkload), so this branch is not normally
	// reached from that path -- but resume-on-registration calls
	// StartWorkload(existing=true) for every persisted record without a
	// prior Stop, and a caller-chosen id can legitimately be reused (see
	// nex-workload-verbs plan D2). Either path hitting an id this agent
	// already holds must not accumulate a duplicate entry.
	workloads := a.Workloads.State[startRequest.Request.Namespace]
	replaced := false
	for i, w := range workloads {
		if w.id == workloadId {
			workloads[i] = newEntry
			replaced = true
			break
		}
	}
	if !replaced {
		workloads = append(workloads, newEntry)
	}
	a.Workloads.State[startRequest.Request.Namespace] = workloads

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
	if a.FailStops {
		return errors.New("injected stop failure")
	}
	if a.StopDelay > 0 {
		time.Sleep(a.StopDelay)
	}

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

	resp := models.AgentListWorkloadsResponse{}

	// The system namespace is administrative: a query for it returns
	// workloads across every namespace. Any other namespace only returns
	// workloads that actually live under that map key. When filter is
	// non-empty, only workloads whose id or name matches one of the
	// filter entries are returned.
	for mapNS, workloads := range a.Workloads.State {
		if namespace != models.SystemNamespace && namespace != mapNS {
			continue
		}
		workloadNS := mapNS
		for _, workload := range workloads {
			if len(filter) > 0 && !slices.Contains(filter, workload.id) && !slices.Contains(filter, workload.name) {
				continue
			}
			resp = append(resp, models.WorkloadSummary{
				Id:                workload.id,
				Namespace:         &workloadNS,
				Name:              workload.name,
				Runtime:           time.Since(workload.startTime).String(),
				StartTime:         workload.startTime.Format(time.RFC3339),
				WorkloadType:      a.WorkloadType,
				WorkloadState:     models.WorkloadStateRunning,
				WorkloadLifecycle: "service",
				Metadata:          map[string]string{"extra": "metadata"},
			})
		}
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
