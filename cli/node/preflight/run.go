package preflight

import (
	"github.com/synadia-io/nex/cli/globals"
)

func (p PreflightCmd) Run(cfg globals.Globals) error {
	if cfg.Check {
		return p.Table()
	}
	return nil
}
