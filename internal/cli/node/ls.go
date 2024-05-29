package node

import (
	"context"
	"errors"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/cli/globals"
)

type ListCmd struct{}

func (l ListCmd) Run(ctx context.Context, nc *nats.Conn, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), l.Table())
	}
	return nil
}

func (l ListCmd) Validate() error {
	return nil
}
