package nexnode

import (
	"context"
	"fmt"
	"log/slog"

	nexmodels "github.com/synadia-io/nex/internal/models"
)

func CmdUp(opts *nexmodels.Options, nodeopts *nexmodels.NodeOptions, ctx context.Context, cancel context.CancelFunc, log *slog.Logger) error {
	node, err := NewNode(opts, nodeopts, ctx, cancel, log)
	if err != nil {
		return fmt.Errorf("failed to initialize node: %s", err)
	}

	go node.Start()

	return nil
}

func CmdPreflight(opts *nexmodels.Options, nodeopts *nexmodels.NodeOptions, ctx context.Context, cancel context.CancelFunc, log *slog.Logger) error {
	config, err := LoadNodeConfiguration(nodeopts.ConfigFilepath)
	if err != nil {
		return fmt.Errorf("failed to load configuration file: %s", err)
	}

	config.ForceDepInstall = nodeopts.ForceDepInstall

	err = CheckPrerequisites(config, false)
	if err != nil {
		return fmt.Errorf("preflight checks failed: %s", err)
	}

	return nil
}
