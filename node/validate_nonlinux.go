//go:build !linux

package node

import "errors"

func (nn *nexNode) validateOS() error {
	var errs error
	if nn.microVMMode {
		errs = errors.Join(errs, errors.New("microvm mode is only supported on Linux"))
	}
	return errs
}
