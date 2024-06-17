package preflight

import (
	"log/slog"

	"github.com/synadia-io/nex/internal/models"
)

func preflightInit(config *models.NodeConfiguration, logger *slog.Logger) PreflightError {
	if !config.NoSandbox {
		logger.Error("Darwin host must be configured to run in no-sandbox mode")
		return ErrNoSandboxRequired
	}

	return nil
}
