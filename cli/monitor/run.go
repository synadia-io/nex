package monitor

import (
	"github.com/synadia-io/nex/cli/globals"
)

func (m MonitorOptions) Run(cfg globals.Globals) error {
	if cfg.Check {
		return m.Table()
	}
	return nil
}
