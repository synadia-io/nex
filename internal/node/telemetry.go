package nexnode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/metric"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const defaultServiceName = "nex-node"

type Telemetry struct {
	log   *slog.Logger
	meter metric.Meter

	metricsEnabled  bool
	metricsExporter string
	metricsPort     int

	serviceName string

	allocatedMemoryCounter metric.Int64UpDownCounter
	allocatedVCPUCounter   metric.Int64UpDownCounter
	deployedByteCounter    metric.Int64UpDownCounter

	vmCounter       metric.Int64UpDownCounter
	workloadCounter metric.Int64UpDownCounter

	functionTriggers       metric.Int64Counter
	functionFailedTriggers metric.Int64Counter
	functionRunTimeNano    metric.Int64Counter
}

func NewTelemetry(log *slog.Logger, config *NodeConfiguration) (*Telemetry, error) {
	t := &Telemetry{
		log:             log,
		meter:           nil,
		metricsEnabled:  config.OtelMetrics,
		metricsExporter: config.OtelMetricsExporter,
		metricsPort:     config.OtelMetricsPort,
		serviceName:     defaultServiceName,
	}

	err := t.init()
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (t *Telemetry) init() error {
	var e, err error

	if t.metricsEnabled {
		e = t.initMeterProvider()
		if err != nil {
			err = errors.Join(err, e)
		}
	} else {
		return nil
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

func (t *Telemetry) initMeterProvider() error {
	resource, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(t.serviceName),
			semconv.ServiceVersion(VERSION),
		))

	if err != nil {
		t.log.Warn("failed to create OTel resource", slog.Any("err", err))
		return err
	}

	metricReader, err := t.serveMetrics() // FIXME-- this seems to require some additional discussion-- it may be better suited to live in Node
	if err != nil {
		t.log.Warn("failed to create OTel metrics exporter", slog.Any("err", err))
		return err
	}

	meterProvider := metricsdk.NewMeterProvider(
		metricsdk.WithResource(resource),
		metricsdk.WithReader(
			metricReader,
		),
	)

	defer func() {
		if err := meterProvider.Shutdown(context.Background()); err != nil {
			t.log.Error("failed to shutdown OTel meter provider", slog.Any("err", err))
		}
	}()

	otel.SetMeterProvider(meterProvider)

	t.meter = otel.Meter(t.serviceName)
	if t.meter == nil {
		return errors.New("failed to initialize telemetry instance: nil meter")
	}

	return nil
}

func (t *Telemetry) serveMetrics() (metricsdk.Reader, error) {
	switch t.metricsExporter {
	case "prometheus":
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
