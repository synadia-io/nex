package node

import (
	"errors"
	"net"
	"os"
	"os/exec"
)

func (nn *nexNode) validateOS() error {
	var errs error
	if nn.microVMMode {
		_, err := exec.LookPath("firecracker")
		if err != nil {
			errs = errors.Join(errs, errors.New("firecracker binary not found in path"))
		}

		if nn.firecrackerOptions.VcpuCount <= 0 {
			errs = errors.Join(errs, errors.New("firecracker vcpu count must be greater than 0"))
		}

		if nn.firecrackerOptions.MemoryMiB <= 0 {
			errs = errors.Join(errs, errors.New("firecracker memory must be greater than 0"))
		}

		_, _, err = net.ParseCIDR(nn.cniOptions.Subnet)
		if err != nil {
			errs = errors.Join(errs, errors.New("invalid CNI subnet provided"))
		}

		if nn.cniOptions.InterfaceName == "" {
			errs = errors.Join(errs, errors.New("CNI interface name cannot be empty"))
		}

		if nn.cniOptions.NetworkName == "" {
			errs = errors.Join(errs, errors.New("CNI network name cannot be empty"))
		}

		for _, bp := range nn.cniOptions.BinPaths {
			if _, err := os.Stat(bp); os.IsNotExist(err) {
				errs = errors.Join(errs, errors.New("provided CNI binary path does not exist: "+bp))
			}
		}

		// FIX: check for cni plugins here

		// FIX: check for agent binaries here
	}
	return errs
}
