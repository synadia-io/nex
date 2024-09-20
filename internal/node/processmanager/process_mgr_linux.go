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
	intnats *internalnats.InternalNatsServer,
	log *slog.Logger,
	config *models.NodeConfiguration,
	telemetry *observability.Telemetry,
	ctx context.Context,
	poolSize int,
) (ProcessManager, error) {
	if config.AgentPluginPath != nil {
		log.Warn("⚠️  Agent workload provider plugins are enabled on this node. Be sure you only allow trusted providers",
			slog.String("plugin_path", *config.AgentPluginPath),
		)
	}

	if config.NoSandbox {
		log.Warn("⚠️  Sandboxing has been disabled! Workloads are spawned directly by agents")
		log.Warn("⚠️  Do not run untrusted workloads in this mode!")
		return NewSpawningProcessManager(intnats, log, config, telemetry, ctx, poolSize)
	}

	return NewFirecrackerProcessManager(intnats, log, config, telemetry, ctx, poolSize)
}
