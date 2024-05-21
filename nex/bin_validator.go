package main

import (
	"fmt"
	"os"
	"slices"
)

func validateBinary(inFile string) (string, string, error) {
	buf, err := os.ReadFile(inFile)
	if err != nil {
		return "", "", err
	}
	switch {
	case slices.Equal(buf[:4], []byte{0x7f, 0x45, 0x4c, 0x46}):
		switch {
		case slices.Equal(buf[17:19], []byte{0x00, 0x3e}):
			return "linux", "amd64", nil
		case slices.Equal(buf[17:19], []byte{0x00, 0xb7}):
			return "linux", "arm64", nil
		default:
			return "", "", fmt.Errorf("unsupported elf file type")
		}
	case slices.Equal(buf[:2], []byte{0x4d, 0x5a}):
		switch {
		case slices.Equal(buf[132:134], []byte{0x64, 0x86}):
			return "windows", "amd64", nil
		case slices.Equal(buf[132:134], []byte{0x64, 0xaa}):
			return "windows", "arm64", nil
		default:
			return "", "", fmt.Errorf("unsupported windows file type")
		}
	case slices.Equal(buf[:4], []byte{0xcf, 0xfa, 0xed, 0xfe}):
		switch {
		case slices.Equal(buf[4:5], []byte{0x07}):
			return "darwin", "amd64", nil
		case slices.Equal(buf[4:5], []byte{0x0c}):
			return "darwin", "arm64", nil
		default:
			return "", "", fmt.Errorf("unsupported darwin file type")
		}
	default:
		return "", "", fmt.Errorf("unsupported binary file type")
	}
}
