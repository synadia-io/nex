//go:build linux || windows

package main

import (
	"context"
	"log/slog"

	nexnode "github.com/synadia-io/nex/internal/node"
)

func setConditionalCommands() {
	nodeUp = nodes.Command("up", "Starts a Nex node")
	nodeUp.Flag("config", "configuration file for the node").Default("./config.json").StringVar(&NodeOpts.ConfigFilepath)
	nodeUp.Flag("metrics", "enable open telemetry metrics endpoint").Default("false").UnNegatableBoolVar(&NodeOpts.OtelMetrics)
	nodeUp.Flag("metrics_port", "enable open telemetry metrics endpoint").Default("8085").IntVar(&NodeOpts.OtelMetricsPort)
	nodeUp.Flag("otel_metrics_exporter", "OTel exporter for metrics").Default("file").EnumVar(&NodeOpts.OtelMetricsExporter, "file", "prometheus")
	nodeUp.Flag("traces", "enable open telemetry traces").Default("false").UnNegatableBoolVar(&NodeOpts.OtelTraces)
	nodeUp.Flag("otel_traces_exporter", "OTel exporter for traces").Default("file").EnumVar(&NodeOpts.OtelTracesExporter, "file", "grpc", "http")

	nodePreflight = nodes.Command("preflight", "Checks system for node requirements and installs missing")
	nodePreflight.Flag("force", "installs missing dependencies without prompt").Default("false").BoolVar(&NodeOpts.ForceDepInstall)
	nodePreflight.Flag("config", "configuration file for the node").Default("./config.json").StringVar(&NodeOpts.ConfigFilepath)
	nodePreflight.Flag("init", "creates the configuration file if it does not exist").EnumVar(&NodeOpts.PreflightInit, "sandbox", "nosandbox")
}

func RunNodeUp(ctx context.Context, logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(newContext(ctx))
	err := nexnode.CmdUp(Opts, NodeOpts, ctx, cancel, logger)
	if err != nil {
		return err
	}
	<-ctx.Done()

	return nil
}

func RunNodePreflight(ctx context.Context, logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(newContext(ctx))
	return nexnode.CmdPreflight(Opts, NodeOpts, ctx, cancel, logger)
}

func newContext(ctx context.Context) context.Context {
	initData := map[string]string{
		"version":    VERSION,
		"commit":     COMMIT,
		"build_date": BUILDDATE,
	}

	return context.WithValue(ctx, "build_data", initData) //nolint:all
}
