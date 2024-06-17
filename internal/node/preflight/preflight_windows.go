package preflight

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/synadia-io/nex/internal/models"
)

func Preflight(config *models.NodeConfiguration, logger *slog.Logger) PreflightError {
	if !config.NoSandbox {
		logger.Error("Windows host must be configured to run in no-sandbox mode")
		return ErrNoSandboxRequired
	}

	if config.ForceDepInstall {
		return installNexAgent(config, logger)
	}

	_, err := exec.LookPath("nex-agent")
	if err != nil {
		fmt.Printf("⛔ You are missing required dependencies for [%s], do you want to install? [y/N] ", red("nex-agent"))
		inputReader := bufio.NewReader(os.Stdin)
		input, err := inputReader.ReadSlice('\n')
		if err != nil {
			return err
		}
		if strings.ToUpper(string(input)) == "Y\n" {
			return installNexAgent(config, logger)
		}
	}

	return nil
}
