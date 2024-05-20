package up

import (
	"github.com/synadia-io/nex/cli/globals"
)

func (u UpCmd) Run(cfg globals.Globals) error {
	if cfg.Check {
		return u.Table()
	}
	return nil
}
