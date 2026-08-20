//go:build !windows

package native

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/carlmjohnson/be"

	"github.com/synadia-io/nex/internal"
)

func discardReaperLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// spawnGroupLeader starts a long-lived process in its own process group, the
// way a native workload is spawned, and returns its pid plus a waitExit(timeout)
// that reports whether the process has exited (and its wait status). A single
// background Wait reaps the process whenever it dies -- from the reaper or from
// the test cleanup -- so "did it exit?" distinguishes killed from still-running
// (a killed child would otherwise be an unreapable zombie).
func spawnGroupLeader(t *testing.T) (int, func(time.Duration) (syscall.WaitStatus, bool)) {
	t.Helper()

	cmd := exec.Command("sleep", "600")
	cmd.SysProcAttr = internal.SysProcAttr()
	be.NilErr(t, cmd.Start())
	pid := cmd.Process.Pid

	exitCh := make(chan syscall.WaitStatus, 1)
	go func() {
		st, _ := cmd.Process.Wait()
		ws, _ := st.Sys().(syscall.WaitStatus)
		exitCh <- ws
	}()

	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })

	waitExit := func(timeout time.Duration) (syscall.WaitStatus, bool) {
		select {
		case ws := <-exitCh:
			return ws, true
		case <-time.After(timeout):
			return 0, false
		}
	}
	return pid, waitExit
}

// TestReaperKillsRecordedOrphan: a pid recorded by a previous incarnation whose
// live process still matches the recorded start time is group-killed at
// startup.
func TestReaperKillsRecordedOrphan(t *testing.T) {
	dir := t.TempDir()
	logger := discardReaperLogger()

	pid, waitExit := spawnGroupLeader(t)

	// A previous incarnation recorded it; the next startup reaps it.
	newOrphanReaper(dir, "node1", logger).record(pid)
	newOrphanReaper(dir, "node1", logger).reapOrphans()

	ws, exited := waitExit(5 * time.Second)
	be.True(t, exited) // killed, not left running
	be.True(t, ws.Signaled())
	be.Equal(t, syscall.SIGKILL, ws.Signal())
}

// TestReaperSkipsReusedPid: a recorded pid whose live process has a DIFFERENT
// start time (the pid was recycled after the crash) is NOT killed -- the
// start-time guard prevents killing an unrelated process. Removing the guard in
// reaper.go makes this test fail (the process exits).
func TestReaperSkipsReusedPid(t *testing.T) {
	dir := t.TempDir()
	logger := discardReaperLogger()

	pid, waitExit := spawnGroupLeader(t)

	// Plant a record for this pid with a start time that cannot match the live
	// process (1ms after the epoch): this pid now "belongs to someone else".
	pidFile := filepath.Join(dir, "node1.workload-pids")
	be.NilErr(t, os.WriteFile(pidFile, []byte(fmt.Sprintf("%d 1\n", pid)), 0o600))

	newOrphanReaper(dir, "node1", logger).reapOrphans()

	// The guard must have refused the kill: the process is still running.
	_, exited := waitExit(time.Second)
	be.False(t, exited)
}

// TestReaperForgetRemovesPid: a forgotten pid is dropped from the file and not
// reaped later.
func TestReaperForgetRemovesPid(t *testing.T) {
	dir := t.TempDir()
	logger := discardReaperLogger()

	pid, waitExit := spawnGroupLeader(t)

	r := newOrphanReaper(dir, "node1", logger)
	r.record(pid)
	r.forget(pid)

	newOrphanReaper(dir, "node1", logger).reapOrphans()

	_, exited := waitExit(time.Second)
	be.False(t, exited) // nothing left to reap; the process survives
}

func pidFileHas(t *testing.T, path string, pid int) bool {
	t.Helper()
	r := &orphanReaper{path: path, logger: discardReaperLogger()}
	recs, err := r.readLocked()
	if err != nil {
		return false
	}
	for _, rec := range recs {
		if rec.pid == pid {
			return true
		}
	}
	return false
}

// TestStartWorkloadRecordsAndForgetsPid drives the record/forget hooks through
// the real spawn path: a spawned workload's pid lands in the reaper file, and a
// stop removes it.
func TestStartWorkloadRecordsAndForgetsPid(t *testing.T) {
	s := newLifecycleState(t)
	dir := t.TempDir()
	s.reaper = newOrphanReaper(dir, "node1", discardReaperLogger())
	pidFile := filepath.Join(dir, "node1.workload-pids")

	req := serviceRequest(t, "reaped", "600")
	be.NilErr(t, s.AddWorkload(lifecycleNamespace, "wl-reap", req))

	pid := runningPid(t, s, lifecycleNamespace, "wl-reap")
	be.True(t, pidFileHas(t, pidFile, pid)) // recorded on spawn

	be.NilErr(t, s.RemoveWorkload(lifecycleNamespace, "wl-reap"))

	// forget runs in the watcher goroutine after the process is reaped.
	forgotten := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !pidFileHas(t, pidFile, pid) {
			forgotten = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	be.True(t, forgotten)
}

// TestReaperDisabledIsNoop: a reaper with no resource directory does nothing
// and never errors.
func TestReaperDisabledIsNoop(t *testing.T) {
	r := newOrphanReaper("", "node1", discardReaperLogger())
	r.record(1234)
	r.forget(1234)
	r.reapOrphans()
	be.Equal(t, "", r.path)
}
