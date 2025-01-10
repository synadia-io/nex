package internal

import (
	"os"

	"github.com/synadia-labs/nex/models"
)

type AgentProcess struct {
	Config  *models.Agent
	Process *os.Process

	Id       string
	HostNode string

	restartCount int
	state        models.AgentState
}
