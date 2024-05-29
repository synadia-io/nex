package stop

import (
	"context"
	"errors"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/cli/globals"
)

type StopOptions struct {
	WorkloadId           string `required:"" placeholder:"<workload_id>" help:"ID of the workload to stop" group:"Stop Configuration" json:"stop_workload_id"`
	TargetNode           string `required:"" placeholder:"SDERPMON..." help:"Node to stop the workload on" group:"Stop Configuration" json:"stop_target_node"`
	ClaimsIssuerFilePath string `required:"" placeholder:"./path/to/nkey.nk" help:"Path to the claims issuer nkey file." group:"Stop Configuration" json:"stop_claims_issuer_file"`
}

func (s StopOptions) Run(ctx context.Context, nc *nats.Conn, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), s.Table())
	}
	return nil
}

func (s StopOptions) Validate() error {
	return nil
}
