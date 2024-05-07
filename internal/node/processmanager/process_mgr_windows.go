//go:build windows

package processmanager

import (
	"context"
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
	return NewSpawningProcessManager(log, config, telemetry, ctx)
}
