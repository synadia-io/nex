//go:build !windows

package internal

import (
	"errors"
	"os"
	"syscall"
)

// SysProcAttr puts each spawned process in its own process group (Setpgid, so
// the child's process-group id equals its pid). That is what lets
// StopProcess/KillProcess signal the WHOLE group -- the workload plus anything
// it forked, e.g. a shell wrapper and the `sleep` it spawns -- instead of only
// the direct child. Signalling just the child left forked grandchildren
// running as orphans after a stop, which is how a "stopped" workload kept
// publishing.
func SysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// StopProcess asks a process group to exit gracefully (SIGINT to the group).
func StopProcess(proc *os.Process) error {
	return signalGroup(proc, syscall.SIGINT)
}

// KillProcess forcibly terminates a process group (SIGKILL to the group). It
// is the backstop for a workload that ignores SIGINT (a shell waiting on a
// child does) -- the group kill takes the child down with the leader rather
// than orphaning it.
func KillProcess(proc *os.Process) error {
	return signalGroup(proc, syscall.SIGKILL)
}

// ProcessGroupAlive reports whether any member of proc's process group is
// still present. A grandchild that ignored the graceful signal keeps the group
// (and its group-id) alive after the leader has exited, so this is how a stop
// decides whether it still has survivors to sweep. Once the last member is
// gone the group-id yields ESRCH.
func ProcessGroupAlive(proc *os.Process) bool {
	if proc == nil {
		return false
	}
	return syscall.Kill(-proc.Pid, 0) == nil
}

// signalGroup sends sig to the process group led by proc. A negative pid
// targets the group whose id is that pid -- proc's own group, since it was
// started with Setpgid. A group that no longer exists (already reaped) is
// reported as os.ErrProcessDone so callers can treat it as "already gone",
// matching the os.Process API they used before.
func signalGroup(proc *os.Process, sig syscall.Signal) error {
	if proc == nil {
		return os.ErrProcessDone
	}
	if err := syscall.Kill(-proc.Pid, sig); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return nil
}
