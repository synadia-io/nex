package processmanager

import (
	"log/slog"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

func (s *SpawningProcessManager) kill(proc *spawnedProcess) error {
	if proc.cmd.Process != nil {
		dll, err := syscall.LoadDLL("kernel32.dll")
		if err != nil {
			return err
		}

		p, err := dll.FindProc("GenerateConsoleCtrlEvent")
		if err != nil {
			return err
		}

		_, _, err = p.Call(syscall.CTRL_BREAK_EVENT, uintptr(proc.cmd.Process.Pid)) // err is always non-nil
		if err != syscall.Errno(0) {
			s.log.Error("Failed to interrupt agent process",
				slog.String("agent_id", proc.ID),
				slog.Int("pid", proc.cmd.Process.Pid),
				slog.String("err", err.Error()),
			)

			return proc.cmd.Process.Signal(os.Kill)
		}
	}

	return nil
}

func (s *SpawningProcessManager) sysProcAttr() *syscall.SysProcAttr {
	return &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
}
