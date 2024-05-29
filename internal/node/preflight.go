package nexnode

import (
	"context"
	"errors"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/cli/globals"
)

type PreflightCmd struct {
	ForceDepInstall bool `name:"force" default:"false" help:"Install missing dependencies without prompt" json:"preflight_force"`
}

func (p PreflightCmd) Run(ctx context.Context, nc *nats.Conn, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), p.Table())
	}
	return nil
}

func (p PreflightCmd) Validate() error {
	return nil
}
