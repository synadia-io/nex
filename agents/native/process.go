package native

import (
	"context"
	"errors"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/synadia-io/nex/models"
)

type NativeProcesses map[string]*NativeProcess

// NativeProcess is one generation of a workload: a single spawned process plus
// the definition it was spawned from. A workload id that is restarted or
// replaced gets a brand new NativeProcess rather than reusing this one, so a
// stop or a watcher holding this pointer can always tell -- by comparing it
// against what the state map holds -- whether the generation it is acting on
// is still the current one.
//
// Restarts is guarded by nexletState's lock, not by this struct's; everything
// else here is either immutable after construction (Name, StartRequest,
// StartedAt, MaxRestarts, cancel, exited) or guarded by this struct's lock
// (Process, State).
type NativeProcess struct {
	sync.RWMutex
	cancel context.CancelFunc

	// exited is closed by the generation's watcher once its process has been
	// waited on. A stop blocks on this rather than guessing.
	exited chan struct{}

	Process      *os.Process
	Name         string
	StartRequest models.StartWorkloadRequest
	StartedAt    time.Time
	State        models.WorkloadState
	Restarts     int
	MaxRestarts  int
}

func (n *NativeProcess) SetState(inState models.WorkloadState) {
	n.Lock()
	defer n.Unlock()

	n.State = inState
}

func (n *NativeProcess) GetState() models.WorkloadState {
	n.RLock()
	defer n.RUnlock()

	return n.State
}

func (n *NativeProcess) setProcess(proc *os.Process) {
	n.Lock()
	defer n.Unlock()

	n.Process = proc
}

func (n *NativeProcess) getProcess() *os.Process {
	n.RLock()
	defer n.RUnlock()

	return n.Process
}

// waitExit blocks until this generation's process is confirmed gone, or until
// timeout elapses; it reports whether the process is gone. The exited channel
// is closed by the watcher as soon as it has reaped the process, so the common
// case returns immediately on exit; the signal poll is the backstop for a
// generation with no watcher running.
func (n *NativeProcess) waitExit(timeout time.Duration) bool {
	proc := n.getProcess()
	if proc == nil {
		return true
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(stopPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.exited:
			return true
		case <-ticker.C:
			if processDone(proc) {
				return true
			}
		case <-deadline:
			return processDone(proc)
		}
	}
}

// isRunning reports whether this generation still holds a live process. A
// generation that has been claimed but not yet spawned counts as running: the
// workload id is taken either way.
func (n *NativeProcess) isRunning() bool {
	if n == nil {
		return false
	}

	switch n.GetState() {
	case models.WorkloadStateStopping, models.WorkloadStateStopped:
		return false
	}

	proc := n.getProcess()
	if proc == nil {
		return n.GetState() == models.WorkloadStateStarting
	}

	select {
	case <-n.exited:
		return false
	default:
	}

	return !processDone(proc)
}

// processDone reports whether the process has exited and been reaped. Signal 0
// is delivered to no one; it only asks whether the process is still there.
func processDone(proc *os.Process) bool {
	return errors.Is(proc.Signal(syscall.Signal(0)), os.ErrProcessDone)
}
