//go:build !windows

package native

import (
	"os"
	"syscall"
)

func stopProcess(proc *os.Process) error {
	return proc.Signal(os.Interrupt)
}

func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
