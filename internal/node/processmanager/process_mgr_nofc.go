//go:build windows || darwin

package processmanager

import (
	"context"
	"log/slog"

	"github.com/synadia-io/nex/internal/models"
	internalnats "github.com/synadia-io/nex/internal/node/internal-nats"
	"github.com/synadia-io/nex/internal/node/observability"
)

// Initialize an appropriate agent process manager instance based on the sandbox config value
func NewProcessManager(
	intNats *internalnats.InternalNatsServer,
	log *slog.Logger,
	config *models.NodeConfiguration,
	telemetry *observability.Telemetry,
	ctx context.Context,
) (ProcessManager, error) {
	return NewSpawningProcessManager(intNats, log, config, telemetry, ctx)
}
