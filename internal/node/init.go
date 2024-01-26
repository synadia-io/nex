package nexnode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/synadia-io/nex/internal/models"
	nexmodels "github.com/synadia-io/nex/internal/models"
)

var (
	nexNodeMeter = otel.Meter("nex-node")

	workloadCounter        metric.Int64UpDownCounter
	deployedByteCounter    metric.Int64UpDownCounter
	allocatedMemoryCounter metric.Int64UpDownCounter
	allocatedVCPUCounter   metric.Int64UpDownCounter

	namespaceWorkloadCounter        map[string]metric.Int64UpDownCounter
	namespaceDeployedByteCounter    map[string]metric.Int64UpDownCounter
	namespaceAllocatedMemoryCounter map[string]metric.Int64UpDownCounter
	namespaceAllocatedVCPUCounter   map[string]metric.Int64UpDownCounter
)

func CmdUp(opts *nexmodels.Options, nodeopts *nexmodels.NodeOptions, ctx context.Context, cancel context.CancelFunc, log *slog.Logger) error {
	namespaceWorkloadCounter = make(map[string]metric.Int64UpDownCounter)
	namespaceDeployedByteCounter = make(map[string]metric.Int64UpDownCounter)
	namespaceAllocatedMemoryCounter = make(map[string]metric.Int64UpDownCounter)
	namespaceAllocatedVCPUCounter = make(map[string]metric.Int64UpDownCounter)

	err := createCounters()
	if err != nil {
		log.Warn("failed to create all OTel counters", slog.Any("err", err))
	}

	nc, err := models.GenerateConnectionFromOpts(opts)
	if err != nil {
		log.Error("Failed to connect to NATS server", slog.Any("err", err))
		return fmt.Errorf("failed to connect to NATS server: %s", err)
	}

	log.Info("Established node NATS connection", slog.String("servers", opts.Servers))

	config, err := LoadNodeConfiguration(nodeopts.ConfigFilepath)
	if err != nil {
		log.Error("Failed to load node configuration file", slog.Any("err", err), slog.String("config_path", nodeopts.ConfigFilepath))
		return fmt.Errorf("failed to load node configuration file: %s", err)
	}

	log.Info("Loaded node configuration from '%s'", slog.String("config_path", nodeopts.ConfigFilepath))

	manager, err := NewMachineManager(ctx, cancel, nc, config, log)
	if err != nil {
		log.Error("Failed to initialize machine manager", slog.Any("err", err))
		return fmt.Errorf("failed to initialize machine manager: %s", err)
	}

	err = manager.Start()
	if err != nil {
		log.Error("Failed to start machine manager", slog.Any("err", err))
		return fmt.Errorf("failed to start machine manager: %s", err)
	}

	return nil
}

func CmdPreflight(opts *nexmodels.Options, nodeopts *nexmodels.NodeOptions, ctx context.Context, cancel context.CancelFunc, log *slog.Logger) error {
	config, err := LoadNodeConfiguration(nodeopts.ConfigFilepath)
	if err != nil {
		return fmt.Errorf("failed to load configuration file: %s", err)
	}

	config.ForceDepInstall = nodeopts.ForceDepInstall

	err = CheckPreRequisites(config)
	if err != nil {
		return fmt.Errorf("preflight checks failed: %s", err)
	}

	return nil
}

func createCounters() error {
	var e, err error
	workloadCounter, e = nexNodeMeter.
		Int64UpDownCounter("nex-workload-count",
			metric.WithDescription("Number of workloads running"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	deployedByteCounter, e = nexNodeMeter.
		Int64UpDownCounter("nex-deployed-bytes-count",
			metric.WithDescription("Total number of bytes deployed"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	allocatedMemoryCounter, e = nexNodeMeter.
		Int64UpDownCounter("nex-total-memory-allocation-mib-count",
			metric.WithDescription("Total allocated memory based on firecracker config"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	allocatedVCPUCounter, e = nexNodeMeter.
		Int64UpDownCounter("nex-total-vcpu-allocation-count",
			metric.WithDescription("Total allocated VCPU based on firecracker config"),
		)
	if e != nil {
		err = errors.Join(err, e)
	}
	return err
}
