package nexnode

import (
	"context"
	"errors"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/cli/globals"
)

type InfoCmd struct {
	Id string `arg:"" required:""`
}

func (i InfoCmd) Run(ctx context.Context, nc *nats.Conn, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), i.Table())
	}
	return nil
}

func (i InfoCmd) Validate() error {
	return nil
}
