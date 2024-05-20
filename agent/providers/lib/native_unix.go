//go:build !windows

package lib

import (
	"fmt"
	"os"
	"syscall"
)

// Undeploy the ELF binary
func (e *NativeExecutable) Undeploy() error {
	e.undeploy.Do(func() {
		defer func() {
			e.removeWorkload()
		}()
		err := e.cmd.Process.Signal(os.Interrupt)

		if err != nil {
			fmt.Println("Couldn't terminate elf binary process")
			e.fail <- true
		}
	})

	return nil
}

func (e *NativeExecutable) sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
