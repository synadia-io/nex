package rootfs

import (
	"github.com/synadia-io/nex/cli/globals"
)

func (r RootfsOptions) Run(cfg globals.Globals) error {
	if cfg.Check {
		return r.Table()
	}
	return nil
}
