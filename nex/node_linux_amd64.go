package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	nexnode "github.com/synadia-io/nex/internal/node"
)

func setConditionalCommands() {
	node_up = nodes.Command("up", "Starts a NEX node")
	node_preflight = nodes.Command("preflight", "Checks system for node requirements and installs missing")

	node_up.Flag("config", "configuration file for the node").Default("./config.json").StringVar(&NodeOpts.ConfigFilepath)
	node_up.Flag("metrics", "enable prometheus metrics endpoint").Default("false").BoolVar(&NodeOpts.PromMetrics)
	node_preflight.Flag("force", "installs missing dependencies without prompt").Default("false").BoolVar(&NodeOpts.ForceDepInstall)
	node_preflight.Flag("config", "configuration file for the node").Default("./config.json").StringVar(&NodeOpts.ConfigFilepath)
}

func RunNodeUp(ctx context.Context, logger *slog.Logger) error {
	if NodeOpts.PromMetrics {
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			logger.Info("prometheus server started", slog.Any("host", "0.0.0.0"), slog.Int("port", 8085))
			err := http.ListenAndServe(":8085", nil)
			if err != nil {
				logger.Error("failed to start metrics server", slog.Any("err", err))
			}
		}()
	}

	ctx, cancel := context.WithCancel(ctx)
	err := nexnode.CmdUp(Opts, NodeOpts, ctx, cancel, logger)
	if err != nil {
		return err
	}
	<-ctx.Done()

	return nil
}

func RunNodePreflight(ctx context.Context, logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	return nexnode.CmdPreflight(Opts, NodeOpts, ctx, cancel, logger)
}
