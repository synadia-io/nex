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
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal"
	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/nex/sdk/go/agent"
)

const (
	// stopGracePeriod is how long a workload has to exit after being asked to
	// stop before it is killed outright.
	stopGracePeriod = 5 * time.Second
	// stopPollInterval is how often a stopping workload's process is
	// re-checked while waiting for it to go away.
	stopPollInterval = 250 * time.Millisecond
	// killConfirmWait is how long a killed process gets to disappear. SIGKILL
	// cannot be caught or ignored, so a process that outlives it is stuck in
	// the kernel and waiting out another full grace period would not free it --
	// it would only push the stop past the node's stop budget.
	killConfirmWait = 3 * stopPollInterval
)

type nexletState struct {
	sync.Mutex

	ctx    context.Context
	logger *slog.Logger
	runner *agent.Runner

	status    models.AgentState
	workloads map[string]NativeProcesses
}

func newNexletState(ctx context.Context, logger *slog.Logger, runner *agent.Runner) *nexletState {
	return &nexletState{
		ctx:       ctx,
		logger:    logger,
		runner:    runner,
		status:    models.AgentStateStarting,
		workloads: make(map[string]NativeProcesses),
	}
}

func (n *nexletState) getWorkload(namespace, workloadId string) *NativeProcess {
	n.Lock()
	defer n.Unlock()

	if ns, ok := n.workloads[namespace]; ok {
		if w, ok := ns[workloadId]; ok {
			return w
		}
	}
	return nil
}

// WorkloadOccupancy reports what is holding a workload id: the generation that
// still has a process on the host -- including one that is being stopped -- and
// whether that generation is running rather than on its way out. Both answers
// come from the same critical section, so a start cannot act on a mix of the two.
func (n *nexletState) WorkloadOccupancy(namespace, workloadId string) (*NativeProcess, bool) {
	n.Lock()
	defer n.Unlock()

	w := n.workloads[namespace][workloadId]
	if !w.isOccupied() {
		return nil, false
	}
	return w, w.isRunning()
}

// deleteGeneration removes the workload id only when workload is still the
// generation stored for it. A stop or an exit that lands after the id has been
// started again belongs to a generation that no longer owns the id, and must
// not delete the entry that replaced it.
func (n *nexletState) deleteGeneration(namespace, workloadId string, workload *NativeProcess) bool {
	n.Lock()
	defer n.Unlock()

	return n.dropGenerationLocked(namespace, workloadId, workload)
}

// dropGenerationLocked is deleteGeneration for callers already holding n's lock.
func (n *nexletState) dropGenerationLocked(namespace, workloadId string, workload *NativeProcess) bool {
	if n.workloads[namespace][workloadId] != workload {
		return false
	}

	delete(n.workloads[namespace], workloadId)
	return true
}

// claimRestart decides, atomically, whether a generation whose process exited
// unexpectedly may restart itself. It refuses when the id has moved on to
// another generation -- restarting then would resurrect a workload that was
// replaced, on top of the one that replaced it -- and when a stop has already
// claimed the generation. On success the restart is recorded against the
// generation so the budget carried into the next start is accurate.
func (n *nexletState) claimRestart(namespace, workloadId string, workload *NativeProcess) bool {
	n.Lock()
	defer n.Unlock()

	if n.workloads[namespace][workloadId] != workload {
		return false
	}
	if workload.GetState() == models.WorkloadStateStopping {
		return false
	}

	workload.SetState(models.WorkloadStateError)
	workload.Restarts++
	return true
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

func (n *nexletState) GetNamespaceWorkloadList(ns string, filter []string) (*models.AgentListWorkloadsResponse, error) {
	n.Lock()
	defer n.Unlock()

	ret := new(models.AgentListWorkloadsResponse)

	// The system namespace is administrative: a query for it returns
	// workloads across every namespace. Any other namespace only returns
	// workloads that actually live under that map key.
	//
	// When filter is non-empty, only workloads whose id or name matches
	// one of the filter entries are returned.
	for mapNS, processes := range n.workloads {
		if ns != models.SystemNamespace && ns != mapNS {
			continue
		}
		workloadNS := mapNS
		for id, w := range processes {
			if len(filter) > 0 && !slices.Contains(filter, id) && !slices.Contains(filter, w.StartRequest.Name) {
				continue
			}
			ws := models.WorkloadSummary{
				Id:                id,
				Namespace:         &workloadNS,
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
	}

	return ret, nil
}

func (n *nexletState) AddWorkload(namespace, workloadId string, req *models.AgentStartWorkloadRequest) error {
	return n.startWorkload(namespace, workloadId, req, nil)
}

// startWorkload spawns a generation of a workload. claimed is the generation
// that claimed this start as its restart (see claimRestart), or nil for a start
// requested from outside. A restart whose claiming generation no longer owns
// the workload id was overtaken by a stop or by a replacement while it was on
// its way in, and is dropped rather than spawned: a restart must never bring
// back a workload the user already stopped, nor run beside the one that
// replaced it.
func (n *nexletState) startWorkload(namespace, workloadId string, req *models.AgentStartWorkloadRequest, claimed *NativeProcess) error {
	n.Lock()
	if _, ok := n.workloads[namespace]; !ok {
		n.workloads[namespace] = make(NativeProcesses)
		n.logger.Debug("namespace created", slog.String("namespace", namespace))
	}

	// The restart budget belongs to the restart chain: it carries over from the
	// generation that claimed the restart, and a start requested from outside
	// gets a fresh one.
	restarts, maxRestarts := 0, MAX_RESTARTS
	if req.Request.WorkloadLifecycle == models.WorkloadLifecycleJob {
		maxRestarts = 1
	}
	if claimed != nil {
		// Identity alone is not enough: a stop that arrived after the restart
		// was claimed still finds the claiming generation under the id, and
		// marks it stopping under this same lock before releasing it. Spawning
		// on top of that would leave the stop reporting success over a workload
		// this restart had just brought back.
		if n.workloads[namespace][workloadId] != claimed || claimed.GetState() == models.WorkloadStateStopping {
			n.Unlock()
			n.logger.Debug("restart abandoned; the claiming generation no longer owns the workload id or is being stopped", slog.String("workloadId", workloadId), slog.String("namespace", namespace))
			return nil
		}
		restarts, maxRestarts = claimed.Restarts, claimed.MaxRestarts

		if restarts >= maxRestarts {
			n.logger.Error("max restarts reached", slog.String("workloadId", workloadId), slog.String("namespace", namespace), slog.Int("restarts", restarts))

			// The claimed generation's process has already exited -- that is
			// why a restart was claimed -- so there is nothing to stop, and
			// dropping it here under the lock that just confirmed its identity
			// is what keeps a start that arrives in the meantime from being
			// torn down in its place. Going by workload id instead would stop
			// and delete whatever holds the id by then.
			n.dropGenerationLocked(namespace, workloadId, claimed)
			n.Unlock()
			claimed.release()

			wse := models.WorkloadStoppedEvent{
				Id:           workloadId,
				Namespace:    namespace,
				WorkloadType: NEXLET_REGISTER_TYPE,
				Error: &models.WorkloadStoppedEventError{
					Code:    "MAX_RESTARTS",
					Message: fmt.Sprintf("workload exhausted its %d restarts", maxRestarts),
				},
			}
			if err := n.runner.EmitEvent(namespace, wse); err != nil {
				n.logger.Error("error emitting workload stopped event", slog.String("err", err.Error()))
			}
			return nil
		}
	}

	poisonPill, cancel := context.WithCancel(n.ctx)

	// Every start -- first start, restart, or a same-id replacement -- gets a
	// fresh NativeProcess. Reusing the struct across generations aliases them
	// all onto one record: a stop already in flight would read the *new*
	// generation's process and kill that instead of the one it was asked to
	// stop, and the definition reported for the id would stay whichever one it
	// was first started with.
	workload := &NativeProcess{
		cancel:       cancel,
		exited:       make(chan struct{}),
		Name:         req.Request.Name,
		StartRequest: req.Request,
		StartedAt:    time.Now(),
		State:        models.WorkloadStateStarting,
		Restarts:     restarts,
		MaxRestarts:  maxRestarts,
	}
	n.workloads[namespace][workloadId] = workload

	startReq := new(StartRequest)
	err := json.Unmarshal([]byte(req.Request.RunRequest), startReq)
	if err != nil {
		n.dropGenerationLocked(namespace, workloadId, workload)
		n.Unlock()
		return err
	}

	if !strings.HasPrefix(startReq.Uri, "file://") && !strings.HasPrefix(startReq.Uri, "nats://") {
		n.dropGenerationLocked(namespace, workloadId, workload)
		n.Unlock()
		return fmt.Errorf("invalid uri; must be prefixed with file:// for local binary or nats:// to fetch an artifact: %s", startReq.Uri)
	}

	var nc *nats.Conn
	if strings.HasPrefix(startReq.Uri, "nats://") {
		nc, err = nats.Connect(strings.Join(req.WorkloadCreds.NatsServers, ","),
			nats.UserJWTAndSeed(req.WorkloadCreds.NatsUserJwt, req.WorkloadCreds.NatsUserSeed),
			nats.Name("artifact_fetcher-"+workloadId))
		if err != nil {
			n.logger.Error("error connecting to nats", slog.String("err", err.Error()))
			n.dropGenerationLocked(namespace, workloadId, workload)
			n.Unlock()
			return fmt.Errorf("failed to make a nats connection to retrieve runnable artifact: %w", err)
		}
	}

	ar, err := getArtifact(startReq.Uri, nc)
	if err != nil {
		n.dropGenerationLocked(namespace, workloadId, workload)
		n.Unlock()
		return fmt.Errorf("failed to retrieve artifact: %w", err)
	}
	if nc != nil {
		nc.Close()
	}
	n.logger.Debug("located artifact", slog.Any("artifact_reference", ar))

	env := []string{}
	for k, v := range startReq.Environment {
		if secretKey, found := strings.CutPrefix(v, models.NexSecretPrefix); found {
			secretValue, err := n.runner.GetNamespaceSecret(namespace, secretKey)
			if err != nil {
				n.dropGenerationLocked(namespace, workloadId, workload)
				n.logger.Error("error retrieving secret", slog.String("err", err.Error()), slog.String("secret_key", secretKey), slog.String("namespace", namespace))
				n.Unlock()
				return fmt.Errorf("failed to retrieve namespace secret %s: %w", secretKey, err)
			}
			env = append(env, k+"="+string(secretValue))
		} else {
			env = append(env, k+"="+v)
		}
	}

	argv := []string{}
	for _, v := range startReq.Argv {
		if secretKey, found := strings.CutPrefix(v, models.NexSecretPrefix); found {
			secretValue, err := n.runner.GetNamespaceSecret(namespace, secretKey)
			if err != nil {
				n.dropGenerationLocked(namespace, workloadId, workload)
				n.logger.Error("error retrieving secret", slog.String("err", err.Error()), slog.String("secret_key", secretKey), slog.String("namespace", namespace))
				n.Unlock()
				return fmt.Errorf("failed to retrieve namespace secret %s: %w", secretKey, err)
			}
			argv = append(argv, string(secretValue))
		} else {
			argv = append(argv, v)
		}
	}

	env = append(env, []string{
		"NEX_WORKLOAD_NATS_SERVERS=" + strings.Join(req.WorkloadCreds.NatsServers, ","),
		"NEX_WORKLOAD_NATS_NKEY=" + req.WorkloadCreds.NatsUserSeed,
		"NEX_WORKLOAD_NATS_B64_JWT=" + base64.StdEncoding.EncodeToString([]byte(req.WorkloadCreds.NatsUserJwt)),
	}...)

	n.logger.Debug("running binary", slog.Any("binary", ar.OriginalURI), slog.Any("args", startReq.Argv))
	cmd := exec.CommandContext(poisonPill, ar.LocalCachePath, argv...)
	cmd.Env = env
	cmd.Stdout = n.runner.GetLogger(workloadId, namespace, models.LogOutStdout)
	cmd.Stderr = n.runner.GetLogger(workloadId, namespace, models.LogOutStderr)
	cmd.SysProcAttr = internal.SysProcAttr()

	if err := cmd.Start(); err != nil {
		n.dropGenerationLocked(namespace, workloadId, workload)
		n.Unlock()
		return fmt.Errorf("failed to start native binary: %w", err)
	}
	workload.setProcess(cmd.Process)
	workload.SetState(models.WorkloadStateRunning)

	go n.watchWorkload(namespace, workloadId, workload, req)

	n.logger.Debug("workload created", slog.String("namespace", namespace), slog.String("workloadId", workloadId), slog.Bool("restart", workload.Restarts > 0))
	n.Unlock()

	if err := n.runner.EmitEvent(namespace, models.WorkloadStartedEvent{Id: workloadId, Namespace: namespace, WorkloadType: NEXLET_REGISTER_TYPE}); err != nil {
		n.logger.Error("error emitting workload stopped event", slog.String("err", err.Error()))
	}
	return nil
}

// watchWorkload owns one generation of a workload from the moment its process
// is running: it waits for that process, publishes the exit, and decides
// whether the workload restarts. Every decision it makes is checked against the
// state map by pointer, because the id may have been stopped and started again
// while this generation was on its way out -- acting on the id blindly would
// then delete or restart on top of a generation this watcher never spawned.
func (n *nexletState) watchWorkload(namespace, workloadId string, workload *NativeProcess, req *models.AgentStartWorkloadRequest) {
	pState, err := workload.getProcess().Wait()

	// A stop blocks on this; close it as soon as the process is reaped,
	// whatever is decided below.
	close(workload.exited)

	exitCode := -1
	if pState != nil {
		exitCode = pState.ExitCode()
	}

	if err != nil {
		n.logger.Error("error waiting on workload process", slog.String("workload_id", workloadId), slog.String("namespace", namespace), slog.String("err", err.Error()))
	} else {
		n.logger.Debug("workload exited without error", slog.String("workload_id", workloadId), slog.String("namespace", namespace), slog.Int("exit_code", exitCode))
	}

	// A stop already in flight owns this generation's teardown: it drops the
	// state entry and emits the stopped event once it has confirmed the exit.
	if workload.GetState() == models.WorkloadStateStopping {
		return
	}

	// A job is expected to exit without user interaction; that is the end of it.
	if err == nil && workload.StartRequest.WorkloadLifecycle == models.WorkloadLifecycleJob {
		if !n.deleteGeneration(namespace, workloadId, workload) {
			return
		}

		wsr := models.WorkloadStoppedEvent{
			Id:           workloadId,
			Namespace:    namespace,
			WorkloadType: NEXLET_REGISTER_TYPE,
		}

		if exitCode != 0 {
			wsr.Error = new(models.WorkloadStoppedEventError)
			wsr.Error.Code = strconv.Itoa(exitCode)
		}

		if err := n.runner.EmitEvent(namespace, wsr); err != nil {
			n.logger.Error("error emitting workload stopped event", slog.String("err", err.Error()))
		}
		return
	}

	if !n.claimRestart(namespace, workloadId, workload) {
		n.logger.Debug("workload process exited but its generation is no longer current; not restarting", slog.String("workloadId", workloadId), slog.String("namespace", namespace))
		return
	}

	n.logger.Debug("workload process exited unexpectedly; attempting restart", slog.String("workloadId", workloadId), slog.String("namespace", namespace), slog.Int("exit_code", exitCode), slog.Int("restarts", workload.Restarts))
	if err := n.startWorkload(namespace, workloadId, req, workload); err != nil {
		n.logger.Error("error restarting workload", slog.String("err", err.Error()))
	}
}

// RemoveWorkload stops a workload and does not return until its process is
// confirmed gone. The node composes UPDATE as stop-confirmed-then-start, so a
// stop that reports success while the process is still running is what leaves
// two generations of the same workload alive and writing to the same log
// subject. A non-nil return means the process could not be confirmed dead --
// the workload stays listed, because it is in fact still there.
func (n *nexletState) RemoveWorkload(namespace, workloadId string) error {
	n.Lock()
	workload, ok := n.workloads[namespace][workloadId]
	if !ok {
		n.Unlock()
		errStr := string(models.GenericErrorsWorkloadNotFound)
		n.logger.Error(errStr, slog.String("workloadId", workloadId), slog.String("namespace", namespace))
		return errors.New(errStr)
	}

	// Claiming the generation happens in the same critical section as the
	// lookup: the stopping state is what keeps this generation's watcher from
	// restarting it out from under the stop.
	workload.SetState(models.WorkloadStateStopping)
	n.Unlock()

	reason, err := n.stopProcess(workload, stopGracePeriod)
	if err != nil {
		n.logger.Error("failed to stop workload", slog.String("workloadId", workloadId), slog.String("namespace", namespace), slog.String("err", err.Error()))
		return err
	}

	if !n.deleteGeneration(namespace, workloadId, workload) {
		// The generation this stop was asked for is gone, but the id was taken
		// again while the stop was running -- only possible once this process
		// was already dead, since a start is refused while one is alive. The
		// stopped event below still goes out, and for the id it now reads false.
		n.logger.Warn("stopped workload was replaced before its stop completed; the stopped event does not describe what holds the id now",
			slog.String("workloadId", workloadId), slog.String("namespace", namespace))
	}

	wse := models.WorkloadStoppedEvent{
		Id:           workloadId,
		Namespace:    namespace,
		WorkloadType: NEXLET_REGISTER_TYPE,
	}
	if reason != "" {
		wse.Error = new(models.WorkloadStoppedEventError)
		wse.Error.Message = reason
	}

	if err := n.runner.EmitEvent(namespace, wse); err != nil {
		n.logger.Error("error emitting workload stopped event", slog.String("err", err.Error()))
	}
	return nil
}

// stopProcess asks a generation's process to exit and does not return until it
// is gone. The returned reason, when non-empty, is what to report on the
// workload stopped event. A non-nil error means the process outlived even the
// kill, and the workload must not be reported as stopped.
func (n *nexletState) stopProcess(workload *NativeProcess, grace time.Duration) (string, error) {
	// This generation is finished either way; releasing its command context
	// retires the exec watchdog that was holding it.
	defer workload.release()

	proc := workload.getProcess()
	if proc == nil {
		// Nothing was ever spawned for this generation.
		return "", nil
	}

	reason := ""
	if err := internal.StopProcess(proc); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			n.logger.Debug("process already exited", slog.Int("pid", proc.Pid))
			return "", nil
		}

		// The signal did not land on a process that is still around;
		// cancelling the command context kills it.
		n.logger.Error("error stopping process; cancelling context", slog.String("err", err.Error()))
		reason = err.Error()
		workload.release()
	}

	if workload.waitExit(grace) {
		return reason, nil
	}

	n.logger.Warn("timeout exceeded waiting for workload to exit; attempting kill", slog.Int("pid", proc.Pid))
	if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		n.logger.Error("error killing process; cancelling context", slog.String("err", err.Error()))
		workload.release()
	}

	if !workload.waitExit(killConfirmWait) {
		return reason, fmt.Errorf("workload process %d did not exit after being killed", proc.Pid)
	}
	return "SYSKILL", nil
}

func (n *nexletState) SetLameduckMode(before time.Duration) error {
	n.Lock()
	defer n.Unlock()

	// Every workload is stopped in parallel, but each stop is the same
	// synchronous stop RemoveWorkload performs, so lameduck does not return
	// until the processes are actually gone.
	var wg sync.WaitGroup
	for namespace, processes := range n.workloads {
		for id, process := range processes {
			wg.Add(1)
			go func(namespace, id string, process *NativeProcess) {
				defer wg.Done()

				process.SetState(models.WorkloadStateStopping)
				reason, err := n.stopProcess(process, before)
				if err != nil {
					n.logger.Error("failed to stop workload during lameduck", slog.String("workloadId", id), slog.String("namespace", namespace), slog.String("err", err.Error()))
					return
				}

				wse := models.WorkloadStoppedEvent{Id: id, Namespace: namespace, WorkloadType: NEXLET_REGISTER_TYPE}
				if reason != "" {
					wse.Error = new(models.WorkloadStoppedEventError)
					wse.Error.Message = reason
				}
				if err := n.runner.EmitEvent(namespace, wse); err != nil {
					n.logger.Error("error emitting workload stopped event", slog.String("err", err.Error()))
				}
			}(namespace, id, process)
		}
	}

	wg.Wait()
	n.workloads = make(map[string]NativeProcesses)
	return nil
}
