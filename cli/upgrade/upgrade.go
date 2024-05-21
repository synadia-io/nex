package upgrade

import (
	"context"
	"errors"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/cli/globals"
)

type UpgradeOptions struct{}

func (u UpgradeOptions) Run(ctx context.Context, nc *nats.Conn, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), u.Table())
	}
	return nil
}

func (u UpgradeOptions) Validate() error {
	return nil
}
