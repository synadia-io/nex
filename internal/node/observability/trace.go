package observability

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func (t *Telemetry) initTrace() error {
	res, err := t.newResource(t.ctx)
	if err != nil {
		return err
	}

	if t.tracesEnabled {
		t.log.Debug("Traces enabled")
		switch t.tracesExporter {
		case "grpc":
			conn, err := grpc.DialContext(t.ctx, t.otelExporterUrl, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
			if err != nil {
				return err
			}
			t.traceExporter, err = otlptracegrpc.New(t.ctx, otlptracegrpc.WithGRPCConn(conn), otlptracegrpc.WithInsecure(), otlptracegrpc.WithEndpoint(t.otelExporterUrl))
			if err != nil {
				return err
			}
			t.log.Info("Initialized OTLP exporter", slog.String("url", t.otelExporterUrl))
		default:
			f, err := os.Create("traces.log")
			if err != nil {
				return err
			}
			t.traceExporter, err = stdouttrace.New(stdouttrace.WithWriter(f))
			if err != nil {
				return err
			}
			t.log.Info("Initialized OTLP exporter", slog.String("file", "traces.log"))
		}
	}

	batchSpanProcessor := tracesdk.NewBatchSpanProcessor(t.traceExporter)
	tracerProvider := tracesdk.NewTracerProvider(
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
		tracesdk.WithResource(res),
		tracesdk.WithSpanProcessor(batchSpanProcessor),
	)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	otel.SetTracerProvider(tracerProvider)
	t.Tracer = otel.Tracer(t.serviceName)
	if t.Tracer == nil {
		return errors.New("failed to initialize telemetry instance: nil tracer")
	}

	return nil
}

func (t *Telemetry) newResource(ctx context.Context) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(t.serviceName),
			semconv.ServiceVersion(*t.version),
			attribute.String("node_pub_key", t.nodePubKey),
			attribute.String("application", t.serviceName),
		),
	)
}
