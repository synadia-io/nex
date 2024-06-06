//go:build linux

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
	intNats *internalnats.InternalNatsServer,
	log *slog.Logger,
	nameserver *string,
	telemetry *observability.Telemetry,
) (ProcessManager, error) {
	if config.NoSandbox {
		log.Warn("⚠️  Sandboxing has been disabled! Workloads are spawned directly by agents")
		log.Warn("⚠️  Do not run untrusted workloads in this mode!")
		return NewSpawningProcessManager(intNats, log, config, telemetry, ctx)
	}

	return NewFirecrackerProcessManager(ctx, config, intNats, log, nameserver, telemetry)
}
