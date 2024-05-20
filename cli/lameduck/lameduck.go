package lameduck

import (
	"github.com/synadia-io/nex/cli/globals"
)

type LameDuckOptions struct{}

func (l LameDuckOptions) Run(cfg globals.Globals) error {
	if cfg.Check {
		return l.Table()
	}
	return nil
}

func (l LameDuckOptions) Validate() error {
	return nil
}
