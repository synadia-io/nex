package main

import (
	"context"
	"log/slog"

	nexnode "github.com/synadia-io/nex/internal/node"
)

func setConditionalCommands() {
	node_up = nodes.Command("up", "Starts a NEX node")
	node_preflight = nodes.Command("preflight", "Checks system for node requirements and installs missing")

	node_up.Flag("config", "configuration file for the node").Default("./config.json").StringVar(&NodeOpts.ConfigFilepath)
	node_preflight.Flag("force", "installs missing dependencies without prompt").Default("false").BoolVar(&NodeOpts.ForceDepInstall)
	node_preflight.Flag("config", "configuration file for the node").Default("./config.json").StringVar(&NodeOpts.ConfigFilepath)
}

func RunNodeUp(ctx context.Context, logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	nexnode.CmdUp(Opts, NodeOpts, ctx, cancel, logger)
	<-ctx.Done()
	return nil
}

func RunNodePreflight(ctx context.Context, logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	nexnode.CmdPreflight(Opts, NodeOpts, ctx, cancel, logger)

	return nil
}
