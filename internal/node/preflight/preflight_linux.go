package preflight

import (
	"log/slog"

	"github.com/synadia-io/nex/internal/models"
)

func preflightInit(_ *models.NodeConfiguration, _ *slog.Logger) PreflightError {
	return nil
}
