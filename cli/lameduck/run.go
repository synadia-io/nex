package lameduck

import (
	"github.com/synadia-io/nex/cli/globals"
)

func (l LameDuckOptions) Run(cfg globals.Globals) error {
	if cfg.Check {
		return l.Table()
	}
	return nil
}
