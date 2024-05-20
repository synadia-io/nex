package monitor

import (
	"context"
	"errors"
	"log/slog"

	"github.com/synadia-io/nex/cli/globals"
)

type LogCmd struct {
	sharedMonitorOptions
}

func (e LogCmd) Run(ctx context.Context, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), e.Table())
	}
	return nil
}

func (m LogCmd) Validate() error {
	return nil
}
