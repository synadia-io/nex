package nexnode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const defaultServiceName = "nex-node"

type Telemetry struct {
	ctx           context.Context
	log           *slog.Logger
	meter         metric.Meter
	meterProvider metric.MeterProvider

	metricsEnabled  bool
	metricsExporter string
	metricsPort     int

	serviceName string
	nodePubKey  string

	allocatedMemoryCounter metric.Int64UpDownCounter
	allocatedVCPUCounter   metric.Int64UpDownCounter
	deployedByteCounter    metric.Int64UpDownCounter

	vmCounter       metric.Int64UpDownCounter
	workloadCounter metric.Int64UpDownCounter

	functionTriggers       metric.Int64Counter
	functionFailedTriggers metric.Int64Counter
	functionRunTimeNano    metric.Int64Counter
}

func NewTelemetry(ctx context.Context, log *slog.Logger, config *NodeConfiguration, nodePubKey string) (*Telemetry, error) {
	t := &Telemetry{
		ctx:             ctx,
		log:             log,
		meter:           nil,
		metricsEnabled:  config.OtelMetrics,
		metricsExporter: config.OtelMetricsExporter,
		metricsPort:     config.OtelMetricsPort,
		serviceName:     defaultServiceName,
		nodePubKey:      nodePubKey,
		meterProvider:   noop.NewMeterProvider(),
	}

	err := t.init()
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (t *Telemetry) init() error {
	var e, err error
	e = t.initMeterProvider()
	if err != nil {
		err = errors.Join(err, e)
	}

	t.vmCounter, e = t.meter.
		Int64UpDownCounter("nex-vm-count",
			metric.WithDescription("Number of VMs started"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	t.workloadCounter, e = t.meter.
		Int64UpDownCounter("nex-workload-count",
			metric.WithDescription("Number of workloads deployed"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	t.deployedByteCounter, e = t.meter.
		Int64UpDownCounter("nex-deployed-bytes-count",
			metric.WithDescription("Total number of bytes deployed"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	t.allocatedMemoryCounter, e = t.meter.
		Int64UpDownCounter("nex-total-memory-allocation-mib-count",
			metric.WithDescription("Total allocated memory based on firecracker config"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	t.allocatedVCPUCounter, e = t.meter.
		Int64UpDownCounter("nex-total-vcpu-allocation-count",
			metric.WithDescription("Total allocated VCPU based on firecracker config"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}

	t.functionTriggers, e = t.meter.
		Int64Counter("nex-function-trigger",
			metric.WithDescription("Total number of times a function was triggered"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	t.functionFailedTriggers, e = t.meter.
		Int64Counter("nex-function-failed-trigger",
			metric.WithDescription("Total number of times a function failed to triggered"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	t.functionRunTimeNano, e = t.meter.
		Int64Counter("nex-function-runtime-nanosec",
			metric.WithDescription("Total run time in nanoseconds for function"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}

	return err
}

func (t *Telemetry) Shutdown() error {
	if _, ok := t.meterProvider.(*metricsdk.MeterProvider); ok {
		return t.meterProvider.(*metricsdk.MeterProvider).Shutdown(t.ctx)
	}
	return nil
}

func (t *Telemetry) initMeterProvider() error {
	if t.metricsEnabled {
		t.log.Debug("Metrics enabled")

		resource, err := resource.Merge(resource.Default(),
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceName(t.serviceName),
				semconv.ServiceVersion(VERSION),
				attribute.String("node_pub_key", t.nodePubKey),
			))

		if err != nil {
			t.log.Warn("failed to create OTel resource", slog.Any("err", err))
			return err
		}

		metricReader, err := t.serveMetrics()
		if err != nil {
			t.log.Warn("failed to create OTel metrics exporter", slog.Any("err", err))
			return err
		}

		t.meterProvider = metricsdk.NewMeterProvider(
			metricsdk.WithResource(resource),
			metricsdk.WithReader(
				metricReader,
			),
		)
	}

	otel.SetMeterProvider(t.meterProvider)

	t.meter = otel.Meter(t.serviceName)
	if t.meter == nil {
		return errors.New("failed to initialize telemetry instance: nil meter")
	}

	return nil
}

func (t *Telemetry) serveMetrics() (metricsdk.Reader, error) {
	switch t.metricsExporter {
	case "prometheus":
		t.log.Debug("Starting prometheus exporter")
		go func() {
			t.log.Info(fmt.Sprintf("serving metrics at localhost:%d/metrics", t.metricsPort))
			http.Handle("/metrics", promhttp.Handler())
			err := http.ListenAndServe(fmt.Sprintf(":%d", t.metricsPort), nil)
			if err != nil {
				t.log.Warn("failed to start prometheus web server", slog.Any("err", err))
			}
		}()

		return prometheus.New()
	default:
		t.log.Debug("Starting standard out exporter")
		reader, err := stdoutmetric.New()
		if err != nil {
			return nil, err
		}

		return metricsdk.NewPeriodicReader(
			reader,
			metricsdk.WithInterval(3*time.Second), // FIXME-- make configurable!
		), nil
	}
}
