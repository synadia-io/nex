package node

import (
	"context"
	"errors"
	"log/slog"

	"github.com/synadia-io/nex/cli/globals"
)

type PreflightCmd struct {
	ForceDepInstall bool `name:"force" default:"false" help:"Install missing dependencies without prompt" json:"preflight_force"`
}

func (p PreflightCmd) Run(ctx context.Context, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), p.Table())
	}
	return nil
}

func (p PreflightCmd) Validate() error {
	return nil
}
