package nexnode

import (
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type Telemetry struct {
	meter metric.Meter

	allocatedMemoryCounter metric.Int64UpDownCounter
	allocatedVCPUCounter   metric.Int64UpDownCounter
	deployedByteCounter    metric.Int64UpDownCounter
	workloadCounter        metric.Int64UpDownCounter

	functionTriggers       metric.Int64Counter
	functionFailedTriggers metric.Int64Counter
	functionRunTimeNano    metric.Int64Counter
}

func NewTelemetry() (*Telemetry, error) {
	t := &Telemetry{
		meter: otel.Meter("nex-node"),
	}

	err := t.init()
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (t *Telemetry) init() error {
	if t.meter == nil {
		return errors.New("failed to initialize telemetry instance: nil meter")
	}

	var e, err error
	t.workloadCounter, e = t.meter.
		Int64UpDownCounter("nex-workload-count",
			metric.WithDescription("Number of workloads running"),
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
