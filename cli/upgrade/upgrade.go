package upgrade

import "github.com/synadia-io/nex/cli/globals"

type UpgradeOptions struct{}

func (u UpgradeOptions) Run(cfg globals.Globals) error {
	if cfg.Check {
		return u.Table()
	}
	return nil
}

func (u UpgradeOptions) Validate() error {
	return nil
}
