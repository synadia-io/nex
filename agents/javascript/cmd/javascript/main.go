package main

import (
	agentcommon "github.com/synadia-io/nex/agents/common"
	jsagent "github.com/synadia-io/nex/agents/javascript"
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

	agent, err := agentcommon.NewNexAgent(
		AgentName,
		AgentVersion,
		AgentDescription,
		0,
		jsAgent)
	if err != nil {
		panic(err)
	}

	err = agent.Run()
	if err != nil {
		panic(err)
	}
}
