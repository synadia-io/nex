package main

import (
	"context"
	"errors"
	"log/slog"
)

type nodePreflightCmd struct {
	ForceDepInstall bool `name:"force" default:"false" help:"Install missing dependencies without prompt" json:"preflight_force"`
}

func (p nodePreflightCmd) Run(ctx context.Context, logger *slog.Logger, cfg globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), p.Table())
	}
	return nil
}

func (p nodePreflightCmd) Validate() error {
	return nil
}
