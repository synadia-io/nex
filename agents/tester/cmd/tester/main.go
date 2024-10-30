package main

import (
	"context"
	"log/slog"

	agentcommon "github.com/synadia-io/nex/agents/common"
	agentapigen "github.com/synadia-io/nex/api/agent/go/gen"
)

const (
	AgentName        = "tester"
	AgentVersion     = "0.0.1"
	AgentDescription = "Custom agent used for testing"
)

// Ensure interface is matched
var theAgent agentcommon.AgentCallback = createTestAgent()

func main() {
	slog.Info("Tester agent - Integration/Acceptance tests only!")
	agent, err := agentcommon.NewNexAgent(
		AgentName,
		AgentVersion,
		AgentDescription,
		0,
		slog.Default(),
		theAgent)
	if err != nil {
		panic(err)
	}

	err = agent.Run()
	if err != nil {
		panic(err)
	}
}

type testAgent struct {
	workloads map[string]*agentapigen.StartWorkloadRequestJson
}

func createTestAgent() *testAgent {
	return &testAgent{
		workloads: make(map[string]*agentapigen.StartWorkloadRequestJson),
	}
}

func (a *testAgent) Up() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	slog.Info("Tester agent is up")

	<-ctx.Done()
	return nil
}

func (a *testAgent) Preflight() error {
	slog.Info("Tester agent preflight")
	return nil
}

func (a *testAgent) StartWorkload(req *agentapigen.StartWorkloadRequestJson) error {
	a.workloads[req.WorkloadId] = req
	return nil
}

func (a *testAgent) StopWorkload(req *agentapigen.StopWorkloadRequestJson) error {
	delete(a.workloads, req.WorkloadId)
	return nil
}

func (a *testAgent) ListWorkloads() error {
	return nil
}
