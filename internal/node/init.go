package nexnode

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"github.com/nats-io/jsm.go/natscontext"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/synadia-io/nex/internal/models"
	nexmodels "github.com/synadia-io/nex/internal/models"
)

var (
	nexNodeMeter = otel.Meter("nex-node")

	workloadCounter metric.Int64UpDownCounter
)

func CmdUp(opts *nexmodels.Options, nodeopts *nexmodels.NodeOptions, ctx context.Context, cancel context.CancelFunc, log *slog.Logger) {
	var err error
	workloadCounter, err = nexNodeMeter.
		Int64UpDownCounter("nex-workload-counter",
			metric.WithDescription("Number of workloads running"),
		)
	if err != nil {
		log.Error("Failed to create workload counter", slog.Any("err", err))
	}

	nc, err := generateConnectionFromOpts(opts)
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
