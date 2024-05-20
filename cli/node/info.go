package node

import (
	"context"
	"errors"
	"log/slog"

	"github.com/synadia-io/nex/cli/globals"
)

type InfoCmd struct {
	Id string `arg:"" required:""`
}

func (i InfoCmd) Run(ctx context.Context, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), i.Table())
	}
	return nil
}

func (i InfoCmd) Validate() error {
	return nil
}
