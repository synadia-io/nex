package upgrade

import (
	"context"
	"errors"
	"log/slog"

	"github.com/synadia-io/nex/cli/globals"
)

type UpgradeOptions struct{}

func (u UpgradeOptions) Run(ctx context.Context, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), u.Table())
	}
	return nil
}

func (u UpgradeOptions) Validate() error {
	return nil
}
