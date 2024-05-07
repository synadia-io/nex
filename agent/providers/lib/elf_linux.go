//go:build linux

package lib

import (
	"fmt"
	"os"
)

// Undeploy the ELF binary
func (e *ELF) Undeploy() error {
	e.undeploy.Do(func() {
		err := e.cmd.Process.Signal(os.Interrupt)
		e.removeWorkload()
		if err != nil {
			fmt.Println("Couldn't terminate elf binary process")
			e.fail <- true
		}
	})

	return nil
}
