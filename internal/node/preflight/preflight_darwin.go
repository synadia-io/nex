package preflight

import (
	"fmt"
	"log/slog"

	"github.com/synadia-io/nex/internal/models"
)

func preflightInit(config *models.NodeConfiguration, _ *slog.Logger) ([]*requirement, PreflightError) {
	if !config.NoSandbox {
		return nil, ErrNoSandboxRequired
	}

	required := []*requirement{
		{
			name: "nex-agent", path: config.BinPath, nosandbox: true,
			description: "Nex-agent binary",
			dlUrl:       fmt.Sprintf(nexAgentDarwinTemplate, nexLatestVersion, nexLatestVersion),
			shaUrl:      fmt.Sprintf(nexAgentDarwinURLTemplateSHA256, nexLatestVersion, nexLatestVersion),
			iF:          downloadDirect,
		},
	}

	return required, nil
}
