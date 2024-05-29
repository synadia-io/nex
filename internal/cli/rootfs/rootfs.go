package rootfs

import (
	"errors"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/cli/globals"
	"golang.org/x/net/context"
)

type RootfsOptions struct {
	OutName         string `default:"rootfs.ext4.gz" required:"" help:"Name of the output file" group:"RootFS Configuration" json:"rootfs_outfile"`
	BaseImage       string `default:"synadia/nex-rootfs:alpine" required:"" help:"Base image to use for the rootfs" group:"RootFS Configuration" json:"rootfs_base_image"`
	BuildScriptPath string `placeholder:"script.sh" help:"Base image to use for the rootfs" group:"RootFS Configuration" json:"rootfs_build_script"`
	AgentBinaryPath string `placeholder:"../path/to/nex-agent" required:"" help:"Path to the agent binary" group:"RootFS Configuration" json:"rootfs_agent_binary"`
	RootFSSize      int    `default:"157286400" help:"Size of the rootfs in bytes" group:"RootFS Configuration" json:"rootfs_size"` //150MB default
}

func (r RootfsOptions) Run(ctx context.Context, nc *nats.Conn, logger *slog.Logger, cfg globals.Globals) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), r.Table())
	}
	return nil
}

func (RootfsOptions) Validate() error {
	return nil
}
