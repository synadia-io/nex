package node

import (
	"context"
	"errors"
	"log/slog"

	"github.com/synadia-io/nex/cli/globals"
)

type ListCmd struct{}

func (l ListCmd) Run(ctx context.Context, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), l.Table())
	}
	return nil
}

func (l ListCmd) Validate() error {
	return nil
}
