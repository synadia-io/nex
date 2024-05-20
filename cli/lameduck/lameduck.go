package lameduck

import (
	"context"
	"errors"
	"log/slog"

	"github.com/synadia-io/nex/cli/globals"
)

type LameDuckOptions struct{}

func (l LameDuckOptions) Run(ctx context.Context, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), l.Table())
	}
	return nil
}

func (l LameDuckOptions) Validate() error {
	return nil
}
