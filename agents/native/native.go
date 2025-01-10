package native

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/goombaio/namegenerator"
	"github.com/synadia-labs/nex/internal"
	"github.com/synadia-labs/nex/models"

	"github.com/nats-io/nkeys"
	sdk "github.com/synadia-io/nexlet.go/agent"
)

//go:embed start_request.json
var startRequest string

var funcTimeLimit = 5 * time.Minute

const (
	nativeAgentName    = "native"
	nativeAgentVersion = "0.0.0"

	maxRestarts = 3
)

func NewNativeWorkloadAgent(natsUrl string, nodeId string, logger *slog.Logger) (*sdk.Runner, error) {
	xkp, err := nkeys.CreateCurveKeys()
	if err != nil {
		return nil, err
	}

	agent := &NativeWorkloadAgent{
		ctx:           context.Background(),
		logger:        logger,
		xKeyPair:      xkp,
		agentState:    models.AgentStateRunning,
		workloadState: make(map[string]NativeProcesses),
		supportedLifecycles: []models.WorkloadLifecycle{
			// models.WorkloadLifecycleFunction,
			models.WorkloadLifecycleJob,
			models.WorkloadLifecycleService,
		},
		startedAt:     time.Now(),
		natsServerUrl: natsUrl,
	}

	opts := []sdk.RunnerOpt{
		sdk.WithLogger(logger),
	}

	if nkeys.IsValidPublicServerKey(nodeId) {
		opts = append(opts, sdk.AsLocalAgent(nodeId))
	} else {
		return nil, errors.New("node id is not a valid public server key")
	}

	agent.runner, err = sdk.NewRunner(nativeAgentName, nativeAgentVersion, agent, opts...)
	if err != nil {
		return nil, err
	}

	return agent.runner, nil
}

type NativeProcess struct {
	Process      *os.Process
	Name         string
	Id           string
	StartRequest *models.StartWorkloadRequest
	StartedAt    time.Time
	State        models.WorkloadState
	Restarts     int
	MaxRestarts  int
}

type (
	NativeProcesses map[string]*NativeProcess
	workloadState   map[string]NativeProcesses
)

func (r *workloadState) WorkloadCount() int {
	count := 0
	for _, workloads := range *r {
		count += len(workloads)
	}
	return count
}

func (r *workloadState) FindWorkload(workloadId string) (*NativeProcess, bool) {
	for _, workloads := range *r {
		if workload, ok := workloads[workloadId]; ok {
			return workload, true
		}
	}
	return nil, false
}

type NativeWorkloadAgent struct {
	ctx context.Context
	id  string

	logger              *slog.Logger
	xKeyPair            nkeys.KeyPair
	agentState          models.AgentState
	stateLock           sync.RWMutex
	workloadState       workloadState // map[namespace][workloadid]workloads
	supportedLifecycles []models.WorkloadLifecycle
	natsServerUrl       string
	startedAt           time.Time

	runner *sdk.Runner
}

func (n *NativeWorkloadAgent) Register(assignedID string) (*models.RegisterAgentRequest, error) {
	n.id = assignedID

	xPub, err := n.xKeyPair.PublicKey()
	if err != nil {
		return nil, err
	}
	return &models.RegisterAgentRequest{
		AssignedId:          assignedID,
		Description:         "Runs workloads as subprocesses on the host machine",
		MaxWorkloads:        0,
		Name:                nativeAgentName,
		PublicXkey:          xPub,
		Version:             nativeAgentVersion,
		SupportedLifecycles: n.supportedLifecycles,
		StartRequestSchema:  startRequest,
	}, nil
}

func (n *NativeWorkloadAgent) Heartbeat() (*models.AgentHeartbeat, error) {
	return &models.AgentHeartbeat{
		Data:          "",
		State:         string(n.agentState),
		WorkloadCount: n.workloadState.WorkloadCount(),
	}, nil
}

func (n *NativeWorkloadAgent) StartWorkload(workloadId string, req *models.AgentStartWorkloadRequest, existing bool) (*models.StartWorkloadResponse, error) {
	n.stateLock.Lock()
	defer n.stateLock.Unlock()

	if n.workloadState[req.Request.Namespace] == nil {
		n.workloadState[req.Request.Namespace] = make(NativeProcesses)
	}

	startReq := new(StartRequest)
	err := json.Unmarshal([]byte(req.Request.RunRequest), startReq)
	if err != nil {
		return nil, err
	}

	if req.Request.Name == "" {
		seed := time.Now().UTC().UnixNano()
		nameGenerator := namegenerator.NewNameGenerator(seed)
		req.Request.Name = nameGenerator.Generate()
	}

	if n.workloadState[req.Request.Namespace][workloadId] == nil {
		n.workloadState[req.Request.Namespace][workloadId] = &NativeProcess{
			Id:           workloadId,
			Name:         req.Request.Name,
			StartRequest: &req.Request,
			StartedAt:    time.Now(),
			State:        models.WorkloadStateStarting,
			Restarts:     0,
			MaxRestarts: func() int {
				if req.Request.WorkloadLifecycle == models.WorkloadLifecycleJob {
					return 1
				}
				return maxRestarts
			}(),
		}
	}

	if n.workloadState[req.Request.Namespace][workloadId].Restarts < n.workloadState[req.Request.Namespace][workloadId].MaxRestarts {
		ctx := n.ctx
		if req.Request.WorkloadLifecycle == models.WorkloadLifecycleFunction {
			ctx, _ = context.WithTimeoutCause(n.ctx, funcTimeLimit, errors.New("exceeded allowed function execution time limit"))
		}

		env := []string{}
		for k, v := range startReq.Environment {
			env = append(env, k+"="+v)
		}
		env = append(env, []string{
			"NEX_WORKLOAD_NATS_URL=" + req.WorkloadCreds.NatsUrl,
			"NEX_WORKLOAD_NATS_NKEY=" + req.WorkloadCreds.NatsUserSeed,
			"NEX_WORKLOAD_NATS_B64_JWT=" + base64.StdEncoding.EncodeToString([]byte(req.WorkloadCreds.NatsUserJwt)),
			"NEX_WORKLOAD_AGENT_ID=" + n.id,
		}...)

		cmd := exec.CommandContext(ctx, startReq.Uri, startReq.Argv...)
		cmd.Env = env
		cmd.Stdout = n.runner.GetLogger(workloadId, req.Request.Namespace, false)
		cmd.Stderr = n.runner.GetLogger(workloadId, req.Request.Namespace, true)
		cmd.SysProcAttr = internal.SysProcAttr()

		if err := cmd.Start(); err != nil {
			return nil, err
		}
		n.workloadState[req.Request.Namespace][workloadId].Process = cmd.Process

		go func() {
			wlId := workloadId
			wlReq := req

			pState, err := n.workloadState[wlReq.Request.Namespace][wlId].Process.Wait()
			if err != nil && !errors.Is(err, syscall.ECHILD) {
				n.logger.Error("error waiting for process to exit", slog.Any("err", err))
			}

			if pState.ExitCode() == 0 {
				err = n.runner.EmitEvent(models.WorkloadStoppedEvent{Id: wlId})
				if err != nil {
					n.logger.Error("error emitting event", slog.Any("err", err))
				}
				return
			}

			// This should only hit when the workload was killed via a proper stop request
			if errors.Is(err, syscall.ECHILD) {
				return
			}

			n.workloadState[wlReq.Request.Namespace][wlId].State = models.WorkloadStateError
			// Stopped by user command
			if x, ok := n.workloadState[wlReq.Request.Namespace][wlId]; !ok || x.State == models.WorkloadStateStopping {
				return
			}

			n.workloadState[wlReq.Request.Namespace][wlId].Restarts++
			n.logger.Debug("workload process exited unexpectedly; attempting restart", slog.String("workloadId", wlId), slog.String("namespace", wlReq.Request.Namespace), slog.Any("restarts", n.workloadState[wlReq.Request.Namespace][wlId].Restarts))

			if n.workloadState[wlReq.Request.Namespace][wlId].Restarts == maxRestarts {
				n.logger.Error("max restarts reached", slog.String("workloadId", wlId), slog.String("namespace", wlReq.Request.Namespace))
				delete(n.workloadState[wlReq.Request.Namespace], wlId)
				return
			}

			_, err = n.StartWorkload(wlId, wlReq, false)
			if err != nil {
				n.logger.Error("error restarting workload", slog.Any("err", err))
			}
		}()

		n.workloadState[req.Request.Namespace][workloadId].State = func() models.WorkloadState {
			if req.Request.WorkloadType == string(models.WorkloadLifecycleFunction) {
				return models.WorkloadStateWarm
			}
			return models.WorkloadStateRunning
		}()

		n.logger.Debug("workload started", slog.String("workloadId", workloadId), slog.String("namespace", req.Request.Namespace), slog.Bool("preexisting", existing))
		err = n.runner.EmitEvent(models.WorkloadStartedEvent{Id: workloadId})
		if err != nil {
			n.logger.Error("error emitting event", slog.Any("err", err))
		}

		return &models.StartWorkloadResponse{
			Id:   workloadId,
			Name: req.Request.Name,
		}, nil
	}

	return nil, errors.New("max restarts reached")
}

func (n *NativeWorkloadAgent) StopWorkload(workloadId string, req *models.StopWorkloadRequest) error {
	n.stateLock.Lock()
	defer n.stateLock.Unlock()

	_, ok := n.workloadState[req.Namespace]
	if !ok {
		// If the agent doesnt find the workload, this is a noop
		return nil
	}

	workload, ok := n.workloadState[req.Namespace][workloadId]
	if !ok {
		// If the agent doesnt find the workload, this is a noop
		return nil
	}

	workload.State = models.WorkloadStateStopping
	err := internal.StopProcess(workload.Process)
	if err != nil {
		n.logger.Error("error stopping process; force killing", slog.Any("err", err))
		err = workload.Process.Kill()
		if err != nil {
			return err
		}
	}

	state, err := workload.Process.Wait()
	if err != nil {
		return err
	}

	n.logger.Debug("workload process exited", slog.String("workloadId", workload.Id), slog.String("state", state.String()))
	delete(n.workloadState[req.Namespace], workloadId)

	err = n.runner.EmitEvent(models.WorkloadStoppedEvent{Id: workloadId})
	if err != nil {
		n.logger.Error("error emitting event", slog.Any("err", err))
	}
	return nil
}

func (n *NativeWorkloadAgent) SetLameduck(before time.Duration) error {
	n.stateLock.Lock()
	defer n.stateLock.Unlock()

	var wg sync.WaitGroup
	for _, processes := range n.workloadState {
		wg.Add(len(processes))
		for _, process := range processes {
			process.State = models.WorkloadStateStopping

			err := internal.StopProcess(process.Process)
			if err != nil {
				return err
			}

			go func() {
				msg, err := process.Process.Wait()
				if err != nil {
					n.logger.Error("error waiting for process to exit; force killing", slog.Any("err", err))
					err = process.Process.Kill()
					if err != nil {
						n.logger.Error("error killing process", slog.Any("err", err))
					}
					wg.Done()
					return
				}
				n.logger.Debug("workload process exited", slog.String("workloadId", process.Id), slog.String("state", msg.String()))
				wg.Done()
			}()
		}
	}

	wg.Wait()
	return nil
}

func (n *NativeWorkloadAgent) QueryWorkloads(namespace string, filter []string) (*models.AgentListWorkloadsResponse, error) {
	resp := models.AgentListWorkloadsResponse{}

	workloads, ok := n.workloadState[namespace]
	if !ok {
		return &resp, nil
	}

	for _, workload := range workloads {
		resp = append(resp, models.WorkloadSummary{
			Id:                workload.Id,
			Name:              workload.Name,
			Runtime:           time.Since(workload.StartedAt).String(),
			StartTime:         workload.StartedAt.Format(time.RFC3339),
			WorkloadLifecycle: string(workload.StartRequest.WorkloadLifecycle),
			WorkloadState:     workload.State,
			WorkloadType:      workload.StartRequest.WorkloadType,
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

func (n *NativeWorkloadAgent) Ping() (*models.AgentSummary, error) {
	supportedLifecycles := strings.Builder{}
	for i, lc := range n.supportedLifecycles {
		if i < len(n.supportedLifecycles)-1 {
			supportedLifecycles.WriteString(string(lc) + ",")
		} else {
			supportedLifecycles.WriteString(string(lc))
		}
	}
	ret := &models.AgentSummary{
		Name:                nativeAgentName,
		StartTime:           n.startedAt.Format(time.RFC3339),
		State:               string(n.agentState),
		SupportedLifecycles: supportedLifecycles.String(),
		WorkloadCount:       n.workloadState.WorkloadCount(),
	}

	return ret, nil
}

func (n *NativeWorkloadAgent) PingWorkload(inWorkloadId string) bool {
	_, ok := n.workloadState.FindWorkload(inWorkloadId)
	return ok
}

func (n *NativeWorkloadAgent) GetWorkload(workloadId, targetXkey string) (*models.StartWorkloadRequest, error) {
	workload, ok := n.workloadState.FindWorkload(workloadId)
	if !ok {
		// noop if workload not found
		return nil, nil
	}
	return workload.StartRequest, nil
}
