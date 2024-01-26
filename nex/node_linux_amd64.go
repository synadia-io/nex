package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	nexnode "github.com/synadia-io/nex/internal/node"
)

func setConditionalCommands() {
	node_up = nodes.Command("up", "Starts a NEX node")
	node_preflight = nodes.Command("preflight", "Checks system for node requirements and installs missing")

	node_up.Flag("config", "configuration file for the node").Default("./config.json").StringVar(&NodeOpts.ConfigFilepath)
	node_up.Flag("metrics", "enable open telemetry metrics endpoint").Default("false").UnNegatableBoolVar(&NodeOpts.OtelMetrics)
	node_up.Flag("otel_metrics_exporter", "OTel exporter for metrics").Default("stdout").EnumVar(&NodeOpts.OtelMetricsExporter, "stdout", "prometheus")
	node_preflight.Flag("force", "installs missing dependencies without prompt").Default("false").BoolVar(&NodeOpts.ForceDepInstall)
	node_preflight.Flag("config", "configuration file for the node").Default("./config.json").StringVar(&NodeOpts.ConfigFilepath)
}

func RunNodeUp(ctx context.Context, logger *slog.Logger) error {
	if NodeOpts.OtelMetrics {
		resource, err := resource.Merge(resource.Default(),
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceName("nex-node"),
				semconv.ServiceVersion(VERSION),
			))
		if err != nil {
			logger.Warn("failed to create OTel resource", slog.Any("err", err))
		}

		metricReader, err := createMetricsReader(logger)
		if err != nil {
			logger.Warn("failed to create OTel metrics exporter", slog.Any("err", err))
		}

		meterProvider := metric.NewMeterProvider(
			metric.WithResource(resource),
			metric.WithReader(
				metricReader,
			),
		)

		defer func() {
			if err := meterProvider.Shutdown(context.Background()); err != nil {
				logger.Error("failed to shutdown OTel meter provider", slog.Any("err", err))
			}
		}()

		otel.SetMeterProvider(meterProvider)
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

func createMetricsReader(log *slog.Logger) (metric.Reader, error) {
	switch NodeOpts.OtelMetricsExporter {
	case "prometheus":
		go func() {
			log.Info("serving metrics at localhost:8085/metrics")
			http.Handle("/metrics", promhttp.Handler())
			err := http.ListenAndServe(":8085", nil)
			if err != nil {
				log.Warn("failed to start prometheus web server", slog.Any("err", err))
			}
		}()
		return prometheus.New()
	default:
		reader, err := stdoutmetric.New()
		if err != nil {
			return nil, err
		}
		return metric.NewPeriodicReader(
			reader,
			metric.WithInterval(3*time.Second),
		), nil
	}
}
