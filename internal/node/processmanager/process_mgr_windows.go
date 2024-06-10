//go:build windows

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
	ctx context.Context,
	config *models.NodeConfiguration,
	intnats *internalnats.InternalNatsServer,
	log *slog.Logger,
	_ *string,
	telemetry *observability.Telemetry,
) (ProcessManager, error) {
	return NewSpawningProcessManager(ctx, config, intnats, log, telemetry)
}
