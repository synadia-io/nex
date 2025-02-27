package native

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/goombaio/namegenerator"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nexlet.go/agent"
	"github.com/synadia-labs/nex/models"
)

//go:embed start_request.json
var startRequest string

const (
	NEXLET_NAME          string = "go_exec"
	NEXLET_REGISTER_TYPE string = "native"
	MAX_RESTARTS         int    = 3
)

var (
	_                    agent.Agent = (*NativeAgent)(nil)
	VERSION              string      = "0.0.0"
	SUPPORTED_LIFECYCLES             = []models.WorkloadLifecycle{
		models.WorkloadLifecycleJob,
		models.WorkloadLifecycleService,
	}
)

type NativeAgent struct {
	ctx       context.Context
	xkp       nkeys.KeyPair
	startTime time.Time

	state      *nexletState
	runner     *agent.Runner
	agentState models.AgentState
}

//go:generate go tool github.com/atombender/go-jsonschema --struct-name-from-title --package native --tags json --output gen_start_request.go start_request.json
func NewNativeWorkloadRunner(ctx context.Context, nodeId string, logger *slog.Logger) (*agent.Runner, error) {
	slog.SetDefault(logger)

	da, err := newNativeWorkloadAgent(ctx, nodeId, logger)
	if err != nil {
		return nil, err
	}

	opts := []agent.RunnerOpt{
		agent.WithLogger(logger),
	}

	if !nkeys.IsValidPublicServerKey(nodeId) {
		return nil, errors.New("node id is not a valid public server key")
	}

	da.runner, err = agent.NewRunner(ctx, nodeId, da, opts...)
	if err != nil {
		return nil, err
	}

	da.state = newNexletState(da.ctx, da.runner)
	return da.runner, nil
}

func newNativeWorkloadAgent(ctx context.Context, nodeId string, logger *slog.Logger) (*NativeAgent, error) {
	xkp, err := nkeys.CreateCurveKeys()
	if err != nil {
		return nil, err
	}

	da := &NativeAgent{
		ctx:        ctx,
		xkp:        xkp,
		startTime:  time.Now(),
		state:      new(nexletState),
		agentState: models.AgentStateStarting,
	}

	da.state.ctx = ctx
	da.state.workloads = make(map[string]NativeProcesses)

	return da, nil
}

func (a *NativeAgent) Register() (*models.RegisterAgentRequest, error) {
	a.startTime = time.Now()

	xPub, err := a.xkp.PublicKey()
	if err != nil {
		return nil, err
	}

	return &models.RegisterAgentRequest{
		Description:         "Runs workloads as subprocesses on the host machine",
		MaxWorkloads:        0,
		Name:                NEXLET_NAME,
		RegisterType:        NEXLET_REGISTER_TYPE,
		PublicXkey:          xPub,
		Version:             VERSION,
		SupportedLifecycles: SUPPORTED_LIFECYCLES,
		StartRequestSchema:  startRequest,
	}, nil
}

func (a *NativeAgent) Heartbeat() (*models.AgentHeartbeat, error) {
	stats := struct {
		TotalNamespaces int    `json:"namespace_count"`
		RegisterType    string `json:"register_type"`
	}{
		TotalNamespaces: a.state.NamespaceCount(),
		RegisterType:    NEXLET_REGISTER_TYPE,
	}

	statsB, err := json.Marshal(stats)
	if err != nil {
		return nil, err
	}

	return &models.AgentHeartbeat{
		Data:          string(statsB),
		State:         string(a.agentState),
		WorkloadCount: a.state.WorkloadCount(),
	}, nil
}

func (a *NativeAgent) StartWorkload(workloadId string, req *models.AgentStartWorkloadRequest, existing bool) (*models.StartWorkloadResponse, error) {
	slog.Debug("start workload request received", slog.String("workloadId", workloadId), slog.String("namespace", req.Request.Namespace))

	if req.Request.Name == "" {
		seed := time.Now().UTC().UnixNano()
		nameGenerator := namegenerator.NewNameGenerator(seed)
		req.Request.Name = nameGenerator.Generate()
	}

	err := a.state.AddWorkload(req.Request.Namespace, workloadId, req)
	if err != nil {
		return nil, err
	}

	return &models.StartWorkloadResponse{
		Id:   workloadId,
		Name: req.Request.Name,
	}, nil
}

func (a *NativeAgent) StopWorkload(workloadId string, req *models.StopWorkloadRequest) error {
	return a.state.RemoveWorkload(req.Namespace, workloadId)
}

func (a *NativeAgent) GetWorkload(workloadId, targetXkey string) (*models.StartWorkloadRequest, error) {
	if wl, ok := a.state.Exists(workloadId); ok {
		return wl, nil
	}
	return nil, errors.New("workload not found")
}

func (a *NativeAgent) QueryWorkloads(namespace string, filter []string) (*models.AgentListWorkloadsResponse, error) {
	return a.state.GetNamespaceWorkloadList(namespace)
}

func (a *NativeAgent) SetLameduck(before time.Duration) error {
	return a.state.SetLameduckMode(before)
}

func (a *NativeAgent) Ping() (*models.AgentSummary, error) {
	return &models.AgentSummary{
		Name:                NEXLET_NAME,
		StartTime:           a.startTime.Format(time.RFC3339),
		State:               string(models.AgentStateRunning),
		SupportedLifecycles: "job,service",
		Version:             VERSION,
		WorkloadCount:       a.state.WorkloadCount(),
	}, nil
}

func (a *NativeAgent) PingWorkload(workloadId string) bool {
	_, ok := a.state.Exists(workloadId)
	return ok
}
