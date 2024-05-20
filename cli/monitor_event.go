package main

import (
	"context"
	"errors"
	"log/slog"
)

type monitorEventCmd struct {
}

func (e monitorEventCmd) Run(ctx context.Context, logger *slog.Logger, cfg NexCLI) error {
	if cfg.Global.Check {
		return errors.Join(cfg.Global.Table(), cfg.Monitor.Table(), e.Table())
	}
	return nil
}

func (e monitorEventCmd) Validate() error {
	return nil
}
