package monitor

import (
	"github.com/synadia-io/nex/cli/globals"
)

type LogCmd struct {
	sharedMonitorOptions
}

func (e LogCmd) Run(cfg globals.Globals) error {
	return nil
}

func (m LogCmd) Validate() error {
	return nil
}
