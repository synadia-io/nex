package preflight

import (
	"fmt"
	"log/slog"

	"github.com/synadia-io/nex/internal/models"
)

func preflightInit(nexVer string, config *models.NodeConfiguration, _ *slog.Logger) ([]*requirement, PreflightError) {
	if !config.NoSandbox {
		return nil, ErrNoSandboxRequired
	}

	required := []*requirement{
		{
			name: "nex-agent", path: config.BinPath, nosandbox: true,
			description: "Nex-agent binary",
			dlUrl:       fmt.Sprintf(nexAgentDarwinTemplate, nexVer, nexVer),
			shaUrl:      fmt.Sprintf(nexAgentDarwinURLTemplateSHA256, nexVer, nexVer),
			iF:          downloadDirect,
		},
	}

	return required, nil
}
