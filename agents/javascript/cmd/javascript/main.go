package main

import (
	"fmt"
	"github.com/synadia-io/nex/agents/common"
)

func main() {
	agent, err := agentcommon.NewNexAgent(doUp, doPreflight)
	if err != nil {
		panic(err)
	}

	err = agent.Run()
	if err != nil {
		panic(err)
	}
}

func doUp() error {
	fmt.Println("Nex JavaScript Agent Started")
	return nil
}

func doPreflight() error {
	fmt.Println("Doing JavaScript Agent Preflight Check")
	return nil
}
