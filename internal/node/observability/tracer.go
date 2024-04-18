package observability

import (
	"context"
	"errors"
	"log/slog"

	"github.com/synadia-io/nex/internal/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	tracer trace.Tracer
)

const (
	traceServiceName = "nex-node"
	traceAppName     = "nex"
)

func GetTracer() trace.Tracer {
	return tracer
}

func InitializeTraceProvider(config *models.NodeConfiguration, log *slog.Logger) error {
	if !config.OtelTraces || config.OtlpExporterUrl == nil {
		tracer = noop.NewTracerProvider().Tracer("nex")
		return nil
	}

	// TODO: verify if this is the right context
	ctx := context.Background()

	res, err := newResource(ctx)
	if err != nil {
		return err
	}

	conn, err := grpc.DialContext(ctx, *config.OtlpExporterUrl, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return err
	}

	traceExporter, err := newExporter(ctx, config.OtelTracesExporter, conn, *config.OtlpExporterUrl)
	if err != nil {
		slog.Error("Failed to create trace exporter", slog.Any("err", err))
		return err
	}

	batchSpanProcessor := sdktrace.NewBatchSpanProcessor(traceExporter)
	tracerProvider := newTraceProvider(res, batchSpanProcessor)
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = otel.Tracer("nex-tracer")

	log.Info("Initialized OTLP exporter", slog.String("url", *config.OtlpExporterUrl))

	return nil
}

func newResource(ctx context.Context) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String(traceServiceName),
			attribute.String("application", traceAppName),
		),
	)
}

func newExporter(ctx context.Context, typeExporter string, conn *grpc.ClientConn, exporterUrl string) (*otlptrace.Exporter, error) {
	// TODO: add support for other exporters
	switch typeExporter {
	case "grpc":
		return otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn), otlptracegrpc.WithInsecure(), otlptracegrpc.WithEndpoint(exporterUrl))
	default:
		return nil, errors.New("unsupported exporter type " + typeExporter)
	}
}

func newTraceProvider(res *resource.Resource, bsp sdktrace.SpanProcessor) *sdktrace.TracerProvider {
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)
	return tracerProvider
}
