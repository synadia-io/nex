package run

import (
	"context"
	"errors"
	"log/slog"

	"github.com/synadia-io/nex/cli/globals"
)

type DevRunOptions struct {
	Filename          string `required:"" default:"./path/to/file" type:"existingfile" help:"Path to workload file" group:"DevRun Configuration" json:"devrun_filename"`
	AutoStop          bool   `default:"true" help:"Stop a workload with the same name on a target" group:"DevRun Configuration" json:"devrun_autostop"`
	DevBucketMaxBytes int    `default:"104857600" help:"Max bytes override for when we create the NEXCLIFILES bucket" group:"DevRun Configuration" json:"devrun_bucket_max_bytes"` // 100MB default
	SharedRunOptions
}

func (d DevRunOptions) Run(ctx context.Context, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), d.Table())
	}
	return nil
}

func (d DevRunOptions) Validate() error {
	return nil
}
