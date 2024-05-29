package lameduck

import (
	"context"
	"errors"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/cli/globals"
)

type LameDuckOptions struct{}

func (l LameDuckOptions) Run(ctx context.Context, nc *nats.Conn, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), l.Table())
	}
	return nil
}

func (l LameDuckOptions) Validate() error {
	return nil
}
