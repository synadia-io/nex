package native

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nexlet.go/agent"
	"github.com/synadia-labs/nex/internal"
	"github.com/synadia-labs/nex/models"
)

type nexletState struct {
	sync.Mutex

	ctx    context.Context
	runner *agent.Runner

	status    models.AgentState
	workloads map[string]NativeProcesses
}

func newNexletState(ctx context.Context, runner *agent.Runner) *nexletState {
	return &nexletState{
		ctx:       ctx,
		runner:    runner,
		status:    models.AgentStateStarting,
		workloads: make(map[string]NativeProcesses),
	}
}

func (n *nexletState) Exists(workloadId string) (*models.StartWorkloadRequest, bool) {
	n.Lock()
	defer n.Unlock()

	for _, ns := range n.workloads {
		if wl, ok := ns[workloadId]; ok {
			return &wl.StartRequest, true
		}
	}
	return nil, false
}

func (n *nexletState) NamespaceCount() int {
	n.Lock()
	defer n.Unlock()

	return len(n.workloads)
}

func (n *nexletState) WorkloadCount() int {
	n.Lock()
	defer n.Unlock()

	total := 0
	for _, ns := range n.workloads {
		total += len(ns)
	}
	return total
}

func (n *nexletState) GetNamespaceWorkloadList(ns string) (*models.AgentListWorkloadsResponse, error) {
	n.Lock()
	defer n.Unlock()

	ret := new(models.AgentListWorkloadsResponse)
	namespace, ok := n.workloads[ns]
	if !ok || len(namespace) == 0 {
		return ret, nil
	}

	for id, w := range namespace {
		ws := models.WorkloadSummary{
			Id:                id,
			Metadata:          map[string]string{},
			Name:              w.StartRequest.Name,
			Runtime:           "--",
			StartTime:         w.StartedAt.Format(time.RFC3339),
			WorkloadLifecycle: string(w.StartRequest.WorkloadLifecycle),
			WorkloadState:     w.GetState(),
			WorkloadType:      NEXLET_REGISTER_TYPE,
			Tags:              w.StartRequest.Tags,
		}
		*ret = append(*ret, ws)
	}

	return ret, nil
}

func (n *nexletState) AddWorkload(namespace, workloadId string, req *models.AgentStartWorkloadRequest) error {
	n.Lock()

	startReq := new(StartRequest)
	err := json.Unmarshal([]byte(req.Request.RunRequest), startReq)
	if err != nil {
		n.Unlock()
		return err
	}

	if !strings.HasPrefix(startReq.Uri, "file://") && !strings.HasPrefix(startReq.Uri, "nats://") {
		n.Unlock()
		return errors.New("invalid uri; must be prefixed with file:// or nats://")
	}

	var nc *nats.Conn
	if strings.HasPrefix(startReq.Uri, "nats://") {
		nc, err = nats.Connect(req.WorkloadCreds.NatsUrl,
			nats.UserJWTAndSeed(req.WorkloadCreds.NatsUserJwt, req.WorkloadCreds.NatsUserSeed),
			nats.Name("artifact_fetcher-"+workloadId))
		if err != nil {
			slog.Error("error connecting to nats", slog.String("err", err.Error()))
			return err
		}
	}

	ar, err := getArtifact(startReq.Uri, nc)
	if err != nil {
		n.Unlock()
		return err
	}

	if nc != nil {
		nc.Close()
	}

	slog.Debug("located artifact", slog.Any("artifact_reference", ar))
	if _, ok := n.workloads[namespace]; !ok {
		slog.Debug("namespace created", slog.String("namespace", namespace))
		n.workloads[namespace] = make(NativeProcesses)
	}

	poisonPill, cancel := context.WithCancel(n.ctx)
	if _, ok := n.workloads[namespace][workloadId]; !ok {
		n.workloads[namespace][workloadId] = &NativeProcess{
			cancel:       cancel,
			Name:         req.Request.Name,
			StartRequest: req.Request,
			StartedAt:    time.Now(),
			State:        models.WorkloadStateStarting,
			Restarts:     0,
			MaxRestarts: func() int {
				if req.Request.WorkloadLifecycle == models.WorkloadLifecycleJob {
					return 1
				}
				return MAX_RESTARTS
			}(),
		}
	} else {
		n.workloads[namespace][workloadId].cancel = cancel
	}

	if n.workloads[namespace][workloadId].Restarts < n.workloads[namespace][workloadId].MaxRestarts {
		env := []string{}
		for k, v := range startReq.Environment {
			env = append(env, k+"="+v)
		}

		env = append(env, []string{
			"NEX_WORKLOAD_NATS_URL=" + req.WorkloadCreds.NatsUrl,
			"NEX_WORKLOAD_NATS_NKEY=" + req.WorkloadCreds.NatsUserSeed,
			"NEX_WORKLOAD_NATS_B64_JWT=" + base64.StdEncoding.EncodeToString([]byte(req.WorkloadCreds.NatsUserJwt)),
		}...)

		slog.Debug("running binary", slog.Any("binary", ar.OriginalURI), slog.Any("args", startReq.Argv))
		cmd := exec.CommandContext(poisonPill, ar.LocalCachePath, startReq.Argv...)
		cmd.Env = env
		cmd.Stdout = n.runner.GetLogger(workloadId, namespace, agent.LogTypeStdout)
		cmd.Stderr = n.runner.GetLogger(workloadId, namespace, agent.LogTypeStderr)
		cmd.SysProcAttr = internal.SysProcAttr()

		if err := cmd.Start(); err != nil {
			delete(n.workloads[namespace], workloadId)
			n.Unlock()
			return err
		}
		n.workloads[namespace][workloadId].Process = cmd.Process
		n.workloads[namespace][workloadId].SetState(models.WorkloadStateRunning)

		go func(namespace, workloadId string, req *models.AgentStartWorkloadRequest) {
			workload := n.workloads[namespace][workloadId]
			if pState, err := workload.Process.Wait(); err == nil {
				// can only by true if StopWorkload was called
				if workload.GetState() == models.WorkloadStateStopping || workload.StartRequest.WorkloadLifecycle == models.WorkloadLifecycleJob {
					slog.Debug("workload exited without error", slog.String("workload_id", workloadId), slog.String("namespace", namespace), slog.Any("exit_code", pState.ExitCode()))
					return
				}
			}
			slog.Debug("workload process exited unexpectedly; attempting restart", slog.String("workloadId", workloadId), slog.String("namespace", req.Request.Namespace), slog.Any("restarts", n.workloads[req.Request.Namespace][workloadId].Restarts))
			workload.SetState(models.WorkloadStateError)
			workload.Restarts++

			err = n.AddWorkload(namespace, workloadId, req)
			if err != nil {
				slog.Error("error restarting workload", slog.String("err", err.Error()))
			}
		}(namespace, workloadId, req)

		slog.Debug("workload created", slog.String("namespace", namespace), slog.String("workloadId", workloadId), slog.Bool("restart", n.workloads[namespace][workloadId].Restarts > 0))
		n.Unlock()
		return nil
	}

	slog.Error("max restarts reached", slog.String("workloadId", workloadId), slog.String("namespace", req.Request.Namespace))
	n.Unlock()
	return n.RemoveWorkload(namespace, workloadId)
}

func (n *nexletState) RemoveWorkload(namespace, workloadId string) error {
	n.Lock()
	defer n.Unlock()

	if ns, ok := n.workloads[namespace]; ok {
		if w, ok := ns[workloadId]; ok {
			go func() {
				n.workloads[namespace][workloadId].SetState(models.WorkloadStateStopping)

				err := stopProcess(w.Process)
				if errors.Is(err, os.ErrProcessDone) {
					slog.Debug("process already exited", slog.String("workloadId", workloadId), slog.String("namespace", namespace))
					n.Lock()
					delete(n.workloads[namespace], workloadId)
					n.Unlock()
					return
				}

				if err != nil {
					slog.Error("error stopping process; attempting to cancel context", slog.String("err", err.Error()))
					w.cancel()
					n.Lock()
					delete(n.workloads[namespace], workloadId)
					n.Unlock()
					return
				}

				timeout := time.After(5 * time.Second) // workload must exit within 5 seconds
				ticker := time.NewTicker(250 * time.Millisecond)
				defer ticker.Stop()

				for {
					select {
					case <-timeout:
						slog.Warn("timeout exceeded waiting for workload to exit; attempting kill", slog.String("workloadId", workloadId), slog.String("namespace", namespace))
						if err := w.Process.Kill(); err != nil {
							slog.Error("Error killing process", slog.String("err", err.Error()))
							w.cancel()
						}
						n.Lock()
						delete(n.workloads[namespace], workloadId)
						n.Unlock()
						return
					case <-ticker.C:
						if err := w.Process.Signal(syscall.Signal(0)); err != nil {
							if err := n.runner.EmitEvent(models.WorkloadStoppedEvent{Id: workloadId}); err != nil {
								slog.Error("error emitting workload stopped event", slog.String("err", err.Error()))
							}
							n.Lock()
							delete(n.workloads[namespace], workloadId)
							n.Unlock()
							return
						}
					}
				}
			}()
			return nil
		}
	}

	return errors.New("workload not found")
}

func (n *nexletState) SetLameduckMode(before time.Duration) error {
	n.Lock()
	defer n.Unlock()

	var wg sync.WaitGroup
	for _, processes := range n.workloads {
		wg.Add(len(processes))
		for id, process := range processes {
			go func() {
				process.SetState(models.WorkloadStateStopping)
				err := internal.StopProcess(process.Process)
				if err != nil {
					slog.Error("error stopping process; cancelling context", slog.String("err", err.Error()))
					process.cancel()
				} else {
					timeout := time.After(before)
					ticker := time.NewTicker(250 * time.Millisecond) // Check every 250ms
					defer ticker.Stop()

					for {
						select {
						case <-timeout:
							fmt.Println("Process did not exit within timeout, sending SIGKILL...")
							if err := process.Process.Kill(); err != nil {
								fmt.Println("Error killing process:", err)
								process.cancel()
							}
							wg.Done()
							return
						case <-ticker.C:
							// Check if the process still exists
							if err := process.Process.Signal(syscall.Signal(0)); err != nil {
								if err := n.runner.EmitEvent(models.WorkloadStoppedEvent{Id: id}); err != nil {
									slog.Error("error emitting workload stopped event", slog.String("err", err.Error()))
								}
								wg.Done()
								return
							}
						}
					}
				}
				wg.Done()
			}()
		}
	}

	wg.Wait()
	n.workloads = make(map[string]NativeProcesses)
	return nil
}
