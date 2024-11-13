package agent

import (
	"context"
	"log/slog"

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	slog.Info("JavaScript agent is up")

	<-ctx.Done()
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
