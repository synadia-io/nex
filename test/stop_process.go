//go:build !windows

package test

import "os"

func stopProcess(proc *os.Process) error {
	return proc.Signal(os.Interrupt)
}
