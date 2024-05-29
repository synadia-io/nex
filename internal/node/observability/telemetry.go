package observability

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tnoop "go.opentelemetry.io/otel/trace/noop"
)

const defaultServiceName = "nex-node"

type OtelConfig struct {
	OtelMetrics         bool   `name:"metrics" default:"false" help:"Enables OTel Metrics" json:"up_otel_metrics_enabled"`
	OtelMetricsPort     int    `default:"8085" json:"up_otel_metrics_port"`
	OtelMetricsExporter string `default:"file" enum:"file,prometheus" json:"up_otel_metrics_exporter"`
	OtelTraces          bool   `name:"traces" default:"false" help:"Enables OTel Traces" json:"up_otel_traces_enabled"`
	OtelTracesExporter  string `default:"file" enum:"file,grpc,http" json:"up_otel_traces_exporter"`
	OtlpExporterUrl     string `default:"127.0.0.1:14532" json:"up_otlp_exporter_url"`
}

type Telemetry struct {
	ctx     context.Context
	log     *slog.Logger
	version *string

	otelExporterUrl string

	metricsEnabled  bool
	metricsExporter string
	metricsPort     int
	meter           metric.Meter
	meterProvider   metric.MeterProvider
	traceProvider   trace.TracerProvider

	tracesEnabled  bool
	tracesExporter string
	traceExporter  tracesdk.SpanExporter

	serviceName string
	nodePubKey  string

	AllocatedMemoryCounter metric.Int64UpDownCounter
	AllocatedVCPUCounter   metric.Int64UpDownCounter
	DeployedByteCounter    metric.Int64UpDownCounter

	VmCounter       metric.Int64UpDownCounter
	WorkloadCounter metric.Int64UpDownCounter

	FunctionTriggers       metric.Int64Counter
	FunctionFailedTriggers metric.Int64Counter
	FunctionRunTimeNano    metric.Int64Counter

	Tracer trace.Tracer
}

func NewTelemetry(ctx context.Context, log *slog.Logger, config OtelConfig, nodePubKey string) (*Telemetry, error) {
	t := &Telemetry{
		ctx:             ctx,
		log:             log,
		meter:           nil,
		otelExporterUrl: config.OtlpExporterUrl,
		metricsEnabled:  config.OtelMetrics,
		metricsExporter: config.OtelMetricsExporter,
		metricsPort:     config.OtelMetricsPort,
		tracesEnabled:   config.OtelTraces,
		tracesExporter:  config.OtelTracesExporter,
		serviceName:     defaultServiceName,
		nodePubKey:      nodePubKey,
		meterProvider:   noop.NewMeterProvider(),
		traceProvider:   tnoop.NewTracerProvider(),
	}

	if _version, ok := t.ctx.Value("VERSION").(string); ok {
		t.version = &_version
	}

	err := t.initMetrics()
	if err != nil {
		return nil, err
	}

	err = t.initTrace()
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (t *Telemetry) Shutdown() error {
	if _, ok := t.meterProvider.(*metricsdk.MeterProvider); ok {
		return t.meterProvider.(*metricsdk.MeterProvider).Shutdown(t.ctx)
	}
	return nil
}
