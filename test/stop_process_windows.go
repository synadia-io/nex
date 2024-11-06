package test

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

func stopProcess(proc *os.Process) error {
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
			return proc.Signal(os.Kill)
		}
	}

	return nil
}

func sysProcAttr() *syscall.SysProcAttr {
	return &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
}
