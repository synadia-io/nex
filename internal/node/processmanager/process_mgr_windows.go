//go:build windows

package processmanager

import (
	"context"
	"errors"
	"log/slog"

	"github.com/synadia-io/nex/internal/models"
	"github.com/synadia-io/nex/internal/node/observability"
)

// Initialize an appropriate agent process manager instance based on the sandbox config value
func NewProcessManager(
	log *slog.Logger,
	config *models.NodeConfiguration,
	telemetry *observability.Telemetry,
	ctx context.Context,
) (ProcessManager, error) {
	if config.NoSandbox {
		log.Warn("⚠️  Sandboxing has been disabled! Workloads are spawned directly by agents")
		log.Warn("⚠️  Do not run untrusted workloads in this mode!")
		return NewSpawningProcessManager(log, config, telemetry, ctx)
	}

	return nil, errors.New("Process manager must be configured without sandboxing")
}
