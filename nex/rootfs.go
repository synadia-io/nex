package main

import (
	"context"
	"log/slog"

	rfs "github.com/synadia-io/nex/internal/fc-image"
)

func CreateRootFS(ctx context.Context, logger *slog.Logger) error {
	return rfs.Build(
		RootfsOpts.BuildScriptPath,
		RootfsOpts.BaseImage,
		RootfsOpts.AgentBinaryPath,
		RootfsOpts.RootFSSize,
	)
}
