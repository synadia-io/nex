package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/synadia-io/nex/models"
)

// Coverage for the stop/update races reported against the native nexlet:
// after `nex workload update` the old and the new process both stayed alive
// and both wrote to the workload's log subject, and after a stop the workload
// disappeared from `workload list` while its process kept running.

const lifecycleNamespace = "lifecycle"

func newLifecycleAgent(t testing.TB) *NativeAgent {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner, err := MockRunner(t)
	be.NilErr(t, err)

	a := &NativeAgent{
		ctx:        context.Background(),
		logger:     logger,
		startTime:  time.Now(),
		runner:     runner,
		agentState: models.AgentStateRunning,
	}
	a.state = newNexletState(a.ctx, logger, runner)
	return a
}

func newLifecycleState(t testing.TB) *nexletState {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner, err := MockRunner(t)
	be.NilErr(t, err)

	return newNexletState(context.Background(), logger, runner)
}

// serviceRequest builds a start request that runs `sleep <seconds>`. The name
// and the argv double as a generation marker: two requests for the same
// workload id differ in both, so the definition the nexlet reports and the
// process it actually runs can be told apart.
func serviceRequest(t testing.TB, name, seconds string) *models.AgentStartWorkloadRequest {
	t.Helper()

	sleepPath, err := exec.LookPath("sleep")
	be.NilErr(t, err)

	return &models.AgentStartWorkloadRequest{
		Request: models.StartWorkloadRequest{
			Description:       name,
			Name:              name,
			Namespace:         lifecycleNamespace,
			RunRequest:        fmt.Sprintf(`{"uri":"file://%s","argv":["%s"]}`, sleepPath, seconds),
			WorkloadLifecycle: models.WorkloadLifecycleService,
			WorkloadType:      NEXLET_REGISTER_TYPE,
		},
	}
}

// processGone reports whether the pid is absent from the process table. A
// process that exited but has not been reaped is still present, so this is the
// same question `ps` answers -- which is how the defect was reported.
func processGone(pid int) bool {
	return errors.Is(syscall.Kill(pid, syscall.Signal(0)), syscall.ESRCH)
}

func runningPid(t testing.TB, s *nexletState, namespace, workloadId string) int {
	t.Helper()

	wl := s.getWorkload(namespace, workloadId)
	if wl == nil {
		t.Fatalf("no workload %q in namespace %q", workloadId, namespace)
	}
	proc := wl.getProcess()
	if proc == nil {
		t.Fatalf("workload %q has no process", workloadId)
	}
	return proc.Pid
}

// assertStable re-checks the condition every 50ms for the whole window and
// fails on the first violation. Used where the defect manifests as something
// changing back after a delay (a stale killer firing, a dead generation being
// restarted).
func assertStable(t testing.TB, window time.Duration, condition func() error) {
	t.Helper()

	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		if err := condition(); err != nil {
			t.Fatalf("state changed during observation window: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A stop must not be reported until the process is actually gone. The node
// composes UPDATE as stop-confirmed-then-start, so a stop that replies
// "stopped" while the process still runs is what lets two generations of the
// same workload write to the log subject at once.
func TestStopWorkloadIsSynchronous(t *testing.T) {
	a := newLifecycleAgent(t)

	_, err := a.StartWorkload("wl-sync", serviceRequest(t, "sync-stop", "30"), false)
	be.NilErr(t, err)

	wl := a.state.getWorkload(lifecycleNamespace, "wl-sync")
	be.Nonzero(t, wl)
	proc := wl.getProcess()
	be.Nonzero(t, proc)
	pid := proc.Pid

	be.NilErr(t, a.StopWorkload("wl-sync", &models.StopWorkloadRequest{Namespace: lifecycleNamespace}))

	// No waiting, no polling: the assertions run on the instruction after the
	// stop returns.
	if err := proc.Signal(syscall.Signal(0)); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("process %d was still alive when StopWorkload returned (signal: %v)", pid, err)
	}
	if !processGone(pid) {
		t.Fatalf("process %d is still in the process table after StopWorkload returned", pid)
	}
	be.Equal(t, 0, a.state.WorkloadCount())
	_, ok := a.state.Exists("wl-sync")
	be.False(t, ok)

	// A stopped service is terminal: its watcher must not restart it.
	assertStable(t, time.Second, func() error {
		if a.state.WorkloadCount() != 0 {
			return fmt.Errorf("stopped workload reappeared in state")
		}
		if !processGone(pid) {
			return fmt.Errorf("process %d came back", pid)
		}
		return nil
	})
}

// Stop-then-start under the same workload id -- the shape `nex workload
// update` takes on the node -- must leave exactly one process and one
// definition behind.
func TestSameIdReplaceHasNoDualWriter(t *testing.T) {
	a := newLifecycleAgent(t)
	const id = "wl-replace"

	reqA := serviceRequest(t, "generation-a", "30")
	_, err := a.StartWorkload(id, reqA, false)
	be.NilErr(t, err)

	genA := a.state.getWorkload(lifecycleNamespace, id)
	be.Nonzero(t, genA)
	pidA := runningPid(t, a.state, lifecycleNamespace, id)

	be.NilErr(t, a.StopWorkload(id, &models.StopWorkloadRequest{Namespace: lifecycleNamespace}))

	reqB := serviceRequest(t, "generation-b", "31")
	_, err = a.StartWorkload(id, reqB, false)
	be.NilErr(t, err)

	genB := a.state.getWorkload(lifecycleNamespace, id)
	be.Nonzero(t, genB)
	if genB == genA {
		t.Fatal("the replacement start reused the stopped generation's NativeProcess")
	}
	pidB := runningPid(t, a.state, lifecycleNamespace, id)
	if pidA == pidB {
		t.Fatal("the replacement start did not spawn a new process")
	}

	if !processGone(pidA) {
		t.Fatalf("the replaced process %d is still running alongside %d", pidA, pidB)
	}
	if processGone(pidB) {
		t.Fatalf("the replacement process %d is not running", pidB)
	}

	def, err := a.GetWorkload(id, "")
	be.NilErr(t, err)
	be.Equal(t, "generation-b", def.Name)
	be.Equal(t, reqB.Request.RunRequest, def.RunRequest)
	be.Equal(t, 1, a.state.WorkloadCount())

	// The stopped generation's killer and watcher must not reach across into
	// the replacement: no delayed kill of pidB, no resurrection of pidA, no
	// second map entry.
	assertStable(t, 1500*time.Millisecond, func() error {
		if got := a.state.getWorkload(lifecycleNamespace, id); got != genB {
			return fmt.Errorf("state entry for %q is no longer the replacement generation (%v)", id, got)
		}
		if a.state.WorkloadCount() != 1 {
			return fmt.Errorf("expected exactly one workload, got %d", a.state.WorkloadCount())
		}
		if processGone(pidB) {
			return fmt.Errorf("replacement process %d was killed by the stopped generation", pidB)
		}
		if !processGone(pidA) {
			return fmt.Errorf("replaced process %d was resurrected", pidA)
		}
		return nil
	})

	be.NilErr(t, a.StopWorkload(id, &models.StopWorkloadRequest{Namespace: lifecycleNamespace}))
}

// Starting an id this nexlet is already running is a caller error, except on
// the resume path (existing == true), which must adopt rather than spawn a
// second process for the same id.
func TestStartWorkloadGuardsAlreadyRunningId(t *testing.T) {
	a := newLifecycleAgent(t)
	const id = "wl-guard"

	_, err := a.StartWorkload(id, serviceRequest(t, "guard-a", "30"), false)
	be.NilErr(t, err)

	genA := a.state.getWorkload(lifecycleNamespace, id)
	pidA := runningPid(t, a.state, lifecycleNamespace, id)

	_, err = a.StartWorkload(id, serviceRequest(t, "guard-b", "31"), false)
	if err == nil {
		t.Fatal("expected an error when starting an id that is already running")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Fatalf("expected an \"already running\" error, got: %v", err)
	}
	be.Equal(t, 1, a.state.WorkloadCount())
	be.Equal(t, genA, a.state.getWorkload(lifecycleNamespace, id))
	be.Equal(t, pidA, runningPid(t, a.state, lifecycleNamespace, id))

	resp, err := a.StartWorkload(id, serviceRequest(t, "guard-c", "32"), true)
	be.NilErr(t, err)
	be.Equal(t, "guard-a", resp.Name)
	be.Equal(t, 1, a.state.WorkloadCount())
	be.Equal(t, genA, a.state.getWorkload(lifecycleNamespace, id))
	be.Equal(t, pidA, runningPid(t, a.state, lifecycleNamespace, id))

	be.NilErr(t, a.StopWorkload(id, &models.StopWorkloadRequest{Namespace: lifecycleNamespace}))
}

// A service that exits on its own is restarted until MAX_RESTARTS is
// exhausted, after which the nexlet drops it.
func TestServiceRestartsUntilMaxRestarts(t *testing.T) {
	s := newLifecycleState(t)

	// `sleep 0` returns immediately, so every start is an unexpected exit.
	be.NilErr(t, s.AddWorkload(lifecycleNamespace, "wl-flap", serviceRequest(t, "flapper", "0")))

	waitFor(t, 20*time.Second, func() bool {
		return s.WorkloadCount() == 0
	}, "flapping service to exhaust its restarts and be dropped")

	assertStable(t, time.Second, func() error {
		if s.WorkloadCount() != 0 {
			return fmt.Errorf("workload came back after max restarts")
		}
		return nil
	})
}

// The identity checks that make the delayed paths safe, exercised directly:
// a killer or a watcher holding a generation that has since been replaced must
// not touch the entry that replaced it.
func TestStaleGenerationCannotDeleteOrRestart(t *testing.T) {
	s := newLifecycleState(t)

	stale := &NativeProcess{Name: "stale", State: models.WorkloadStateError, MaxRestarts: MAX_RESTARTS}
	current := &NativeProcess{Name: "current", State: models.WorkloadStateRunning, MaxRestarts: MAX_RESTARTS}

	s.workloads[lifecycleNamespace] = NativeProcesses{"wl-gen": current}

	be.False(t, s.deleteGeneration(lifecycleNamespace, "wl-gen", stale))
	be.Equal(t, current, s.getWorkload(lifecycleNamespace, "wl-gen"))

	be.False(t, s.claimRestart(lifecycleNamespace, "wl-gen", stale))
	be.Equal(t, 0, stale.Restarts)
	be.Equal(t, 0, current.Restarts)
	be.Equal(t, models.WorkloadStateRunning, current.GetState())

	// A restart that reaches the start path after its generation stopped
	// owning the id is dropped rather than spawned beside the replacement.
	be.NilErr(t, s.startWorkload(lifecycleNamespace, "wl-gen", serviceRequest(t, "stale", "30"), stale))
	be.Equal(t, current, s.getWorkload(lifecycleNamespace, "wl-gen"))
	be.Equal(t, 1, s.WorkloadCount())

	// A stop already underway owns the teardown; the watcher must not race it
	// into a restart.
	current.SetState(models.WorkloadStateStopping)
	be.False(t, s.claimRestart(lifecycleNamespace, "wl-gen", current))
	be.Equal(t, 0, current.Restarts)

	// The current generation, not being stopped, does claim the restart.
	current.SetState(models.WorkloadStateRunning)
	be.True(t, s.claimRestart(lifecycleNamespace, "wl-gen", current))
	be.Equal(t, 1, current.Restarts)
	be.Equal(t, models.WorkloadStateError, current.GetState())

	// The stale generation's delete stays a no-op even once the current
	// generation is gone -- it only ever removes what it put there.
	be.True(t, s.deleteGeneration(lifecycleNamespace, "wl-gen", current))
	be.False(t, s.deleteGeneration(lifecycleNamespace, "wl-gen", stale))
	be.Equal(t, 0, s.WorkloadCount())
}
