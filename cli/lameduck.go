package main

import (
	"context"
	"errors"
	"log/slog"
)

type lameDuckOptions struct{}

func (l lameDuckOptions) Run(ctx context.Context, logger *slog.Logger, cfg globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), l.Table())
	}
	return nil
}

func (l lameDuckOptions) Validate() error {
	return nil
}
