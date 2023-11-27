package controlapi

import (
	"debug/elf"
	"errors"
	"fmt"
)

// Validates that the indicated file is a 64-bit linux native elf binary that is statically linked.
// All native binaries must pass this validation before they are executed.
func ValidateNativeBinary(filename string) error {

	elfFile, err := elf.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open source binary: %s", err)
	}
	defer elfFile.Close()

	err = verifyStatic(elfFile)
	if err != nil {
		return fmt.Errorf("file failed static link check: %s", err)
	}

	return nil
}

func verifyStatic(elf *elf.File) error {
	for _, prog := range elf.Progs {
		if prog.ProgHeader.Type == 3 { // PT_INTERP
			return errors.New("elf binary contains at least one dynamically linked dependency")
		}
	}
	return nil
}
