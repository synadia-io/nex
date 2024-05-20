package main

import (
	"context"
	"errors"
	"log/slog"
)

type monitorLogCmd struct {
}

func (l monitorLogCmd) Run(ctx context.Context, logger *slog.Logger, cfg NexCLI) error {
	if cfg.Global.Check {
		return errors.Join(cfg.Global.Table(), cfg.Monitor.Table(), l.Table())
	}
	return nil
}

func (l monitorLogCmd) Validate() error {
	return nil
}
