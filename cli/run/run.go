package run

import (
	"github.com/synadia-io/nex/cli/globals"
)

func (d DevRunOptions) Run(ctx globals.Globals) error {
	return nil
}

func (r RunOptions) Run(cfg globals.Globals) error {
	if cfg.Check {
		return r.Table()
	}
	return nil
}
