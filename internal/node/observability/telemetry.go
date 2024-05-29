package observability

import (
	"context"
	"log/slog"

	"github.com/synadia-io/nex/internal/cli/node"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tnoop "go.opentelemetry.io/otel/trace/noop"
)

const defaultServiceName = "nex-node"

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

func NewTelemetry(ctx context.Context, log *slog.Logger, config *node.OtelConfig, nodePubKey string) (*Telemetry, error) {
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

	if buildData, ok := t.ctx.Value("build_data").(map[string]string); ok {
		if _version, _ok := buildData["version"]; _ok {
			t.version = &_version
		}
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
