//go:build windows

package lib

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/windows"
)

// Undeploy the ELF binary
func (e *ELF) Undeploy() error {
	e.undeploy.Do(func() {
		defer func() {
			e.removeWorkload()
		}()

		dll, err := syscall.LoadDLL("kernel32.dll")
		if err != nil {
			fmt.Printf("Failed to terminate elf binary process; %s\n", err.Error())
			e.fail <- true
			return
		}

		p, err := dll.FindProc("GenerateConsoleCtrlEvent")
		if err != nil {
			fmt.Printf("Failed to terminate elf binary process; %s\n", err.Error())
			e.fail <- true
			return
		}

		_, _, err = p.Call(syscall.CTRL_BREAK_EVENT, uintptr(e.cmd.Process.Pid)) // err is always non-nil
		if err != syscall.Errno(0) {
			fmt.Printf("Failed to terminate elf binary process; %s\n", err.Error())
			e.fail <- true
			return
		}
	})

	return nil
}

func (e *ELF) sysProcAttr() *syscall.SysProcAttr {
	return &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
}
