package monitor

import (
	"github.com/synadia-io/nex/cli/globals"
)

type EventCmd struct {
	sharedMonitorOptions
}

func (e EventCmd) Run(cfg globals.Globals) error {
	return nil
}

func (m EventCmd) Validate() error {
	return nil
}
