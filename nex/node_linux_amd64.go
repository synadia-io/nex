package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/choria-io/fisk"
	nexnode "github.com/synadia-io/nex/internal/node"
)

func init() {
	node_up = nodes.Command("up", "Starts a NEX node")
	node_preflight = nodes.Command("preflight", "Checks system for node requirements and installs missing")
	node_up.Flag("config", "configuration file for the node").Default("./config.json").StringVar(&NodeOpts.ConfigFilepath)
	node_preflight.Flag("force", "installs missing dependencies without prompt").Default("false").BoolVar(&NodeOpts.ForceDepInstall)
	node_preflight.Flag("config", "configuration file for the node").Default("./config.json").StringVar(&NodeOpts.ConfigFilepath)

	ctx := context.Background()
	opts := slog.HandlerOptions{}

	switch Opts.LogLevel {
	case "debug":
		opts.Level = slog.LevelDebug
	case "info":
		opts.Level = slog.LevelInfo
	case "warn":
		opts.Level = slog.LevelWarn
	default:
		opts.Level = slog.LevelError
	}

	var logger *slog.Logger
	if Opts.LogJSON {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &opts))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &opts))
	}

	switch fisk.MustParse(ncli.Parse(os.Args[1:])) {
	case node_up.FullCommand():
		err := RunNodeUp(ctx, logger)
		if err != nil {
			logger.Error("failed to start node", slog.Any("err", err))
		}
	case node_preflight.FullCommand():
		err := RunNodePreflight(ctx, logger)
		if err != nil {
			logger.Error("failed to run node preflight", slog.Any("err", err))
		}
	}
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
	<-ctx.Done()
	return nil
}
