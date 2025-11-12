//go:build !windows

package internal

import (
	"os"
	"syscall"
)

func StopProcess(proc *os.Process) error {
	return proc.Signal(os.Interrupt)
}

func SysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
