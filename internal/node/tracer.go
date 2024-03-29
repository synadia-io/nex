package nexnode

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
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

func InitializeTraceProvider(exporterUrl string) error {
	// TODO: verify if this is the right context
	ctx := context.Background()

	res, err := newResource(ctx)
	if err != nil {
		return err
	}

	conn, err := grpc.DialContext(ctx, exporterUrl, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return err
	}

	traceExporter, err := newExporter(ctx, conn, exporterUrl)
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

func newExporter(ctx context.Context, conn *grpc.ClientConn, exporterUrl string) (*otlptrace.Exporter, error) {
	return otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn), otlptracegrpc.WithInsecure(), otlptracegrpc.WithEndpoint(exporterUrl))
}

func newTraceProvider(res *resource.Resource, bsp sdktrace.SpanProcessor) *sdktrace.TracerProvider {
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)
	return tracerProvider
}
