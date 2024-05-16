package upgrade

import "github.com/synadia-io/nex/cli/globals"

func (u UpgradeOptions) Run(cfg globals.Globals) error {
	if cfg.Check {
		return u.Table()
	}
	return nil
}
