//go:build linux

package processmanager

import (
	"context"
	"log/slog"

	"github.com/synadia-io/nex/internal/cli/node"
	"github.com/synadia-io/nex/internal/node/observability"
)

// Initialize an appropriate agent process manager instance based on the sandbox config value
func NewProcessManager(
	log *slog.Logger,
	config *node.NodeOptions,
	telemetry *observability.Telemetry,
	ctx context.Context,
) (ProcessManager, error) {
	if config.Up.NoSandbox {
		log.Warn("⚠️  Sandboxing has been disabled! Workloads are spawned directly by agents")
		log.Warn("⚠️  Do not run untrusted workloads in this mode!")
		return NewSpawningProcessManager(log, config, telemetry, ctx)
	}

	return NewFirecrackerProcessManager(log, config, telemetry, ctx)
}
