package stop

import (
	"github.com/synadia-io/nex/cli/globals"
)

func (s StopOptions) Run(cfg globals.Globals) error {
	if cfg.Check {
		return s.Table()
	}
	return nil
}
