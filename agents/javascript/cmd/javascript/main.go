package main

import (
	"github.com/synadia-io/nex/agents/common"
	jsagent "github.com/synadia-io/nex/agents/javascript"
	"log/slog"
)

const (
	AgentName        = "javascript"
	AgentVersion     = "0.0.1"
	AgentDescription = "Nex JavaScript execution agent"
)

func main() {

	jsAgent, err := jsagent.NewAgent()
	if err != nil {
		panic(err)
	}

	slog.Info("JavaScript agent starting")
	agent, err := agentcommon.NewNexAgent(
		AgentName,
		AgentVersion,
		AgentDescription,
		0,
		slog.Default(),
		jsAgent)
	if err != nil {
		panic(err)
	}

	err = agent.Run()
	if err != nil {
		panic(err)
	}
}
