package internal

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

func StopProcess(proc *os.Process) error {
	if proc != nil {
		dll, err := syscall.LoadDLL("kernel32.dll")
		if err != nil {
			return err
		}

		p, err := dll.FindProc("GenerateConsoleCtrlEvent")
		if err != nil {
			return err
		}

		_, _, err = p.Call(syscall.CTRL_BREAK_EVENT, uintptr(proc.Pid)) // err is always non-nil
		if err != syscall.Errno(0) {
			return err
		}
	}

	return nil
}

// KillProcess forcibly terminates the process. On Windows this kills the
// process itself; tree termination would require a Job object, which the
// graceful CREATE_NEW_PROCESS_GROUP + CTRL_BREAK path above does not set up.
func KillProcess(proc *os.Process) error {
	if proc == nil {
		return os.ErrProcessDone
	}
	return proc.Kill()
}

// ProcessGroupAlive reports whether the process is still present. Windows has
// no cheap process-group liveness probe here, so this checks the process
// itself; the sweep it guards is a no-op once the process has exited.
func ProcessGroupAlive(proc *os.Process) bool {
	if proc == nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func SysProcAttr() *syscall.SysProcAttr {
	return &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
}
