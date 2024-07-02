package observability

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/metric"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func (t *Telemetry) initMetrics() error {
	var e, err error
	err = t.initMeterProvider()
	if err != nil {
		return err
	}

	t.VmCounter, e = t.meter.
		Int64UpDownCounter("nex-vm-count",
			metric.WithDescription("Number of VMs started"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	t.WorkloadCounter, e = t.meter.
		Int64UpDownCounter("nex-workload-count",
			metric.WithDescription("Number of workloads deployed"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	t.DeployedByteCounter, e = t.meter.
		Int64UpDownCounter("nex-deployed-bytes-count",
			metric.WithDescription("Total number of bytes deployed"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	t.AllocatedMemoryCounter, e = t.meter.
		Int64UpDownCounter("nex-total-memory-allocation-mib-count",
			metric.WithDescription("Total allocated memory based on firecracker config"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	t.AllocatedVCPUCounter, e = t.meter.
		Int64UpDownCounter("nex-total-vcpu-allocation-count",
			metric.WithDescription("Total allocated VCPU based on firecracker config"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}

	t.FunctionTriggers, e = t.meter.
		Int64Counter("nex-function-trigger",
			metric.WithDescription("Total number of times a function was triggered"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	t.FunctionFailedTriggers, e = t.meter.
		Int64Counter("nex-function-failed-trigger",
			metric.WithDescription("Total number of times a function failed to triggered"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	t.FunctionRunTimeNano, e = t.meter.
		Int64Counter("nex-function-runtime-nanosec",
			metric.WithDescription("Total run time in nanoseconds for function"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}

	return err
}

func (t *Telemetry) initMeterProvider() error {
	if t.metricsEnabled {
		t.log.Debug("Metrics enabled")

		if t.version == nil {
			return errors.New("failed to initialize meter provider; no version resolved in context")
		}

		resource, err := resource.Merge(resource.Default(),
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceName(t.serviceName),
				semconv.ServiceVersion(*t.version),
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

	otel.SetMeterProvider(t.meterProvider) // t.meterProvider is a noop.MeterProvider by default

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
		f, err := os.Create("metrics.log")
		if err != nil {
			t.log.Error("Failed to create metrics log file", slog.Any("err", err))
			return nil, err
		}
		reader, err := stdoutmetric.New(stdoutmetric.WithWriter(f))
		if err != nil {
			return nil, err
		}

		return metricsdk.NewPeriodicReader(
			reader,
			metricsdk.WithInterval(3*time.Second), // FIXME-- make configurable!
		), nil
	}
}
