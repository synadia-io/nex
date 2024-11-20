package agent

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	agentcommon "github.com/synadia-io/nex/agents/common"
	agentapigen "github.com/synadia-io/nex/api/agent/go/gen"
)

var _ agentcommon.AgentCallback = &JavaScriptAgent{}

type JavaScriptAgent struct {
	workloads map[string]*agentapigen.StartWorkloadRequestJson
}

func NewAgent() (*JavaScriptAgent, error) {
	return &JavaScriptAgent{
		workloads: make(map[string]*agentapigen.StartWorkloadRequestJson),
	}, nil
}

func (a *JavaScriptAgent) Up() error {
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt)
	fmt.Fprintln(os.Stdout, "JavaScript agent is up")

	<-exit
	fmt.Println("JavaScript agent shutting down")
	return nil
}

func (a *JavaScriptAgent) Preflight() error {
	slog.Info("JavaScript agent preflight")
	return nil
}

func (a *JavaScriptAgent) StartWorkload(req *agentapigen.StartWorkloadRequestJson) error {
	a.workloads[req.WorkloadId] = req
	return nil
}

func (a *JavaScriptAgent) StopWorkload(req *agentapigen.StopWorkloadRequestJson) error {
	delete(a.workloads, req.WorkloadId)
	return nil
}

func (a *JavaScriptAgent) ListWorkloads() error {
	return nil
}

func (a *JavaScriptAgent) Trigger(workloadId string, payload []byte) error {
	return nil
}
