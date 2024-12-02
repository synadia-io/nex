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
	runner    *ScriptRunner
}

func newProvider(workloadId string, allocator vmAllocator) hostServiceProvider {
	return NewNodeHostServicesProvider(workloadId, allocator)
}

func NewAgent() (*JavaScriptAgent, error) {
	return &JavaScriptAgent{
		runner:    NewScriptRunner(newProvider),
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
	// TODO: get script file
	f, err := os.ReadFile(req.LocalFilePath)
	if err != nil {
		return err
	}

	err = a.runner.AddScript(req.WorkloadId, string(f))
	if err != nil {
		return err
	}

	return nil
}

func (a *JavaScriptAgent) StopWorkload(req *agentapigen.StopWorkloadRequestJson) error {
	delete(a.workloads, req.WorkloadId)

	err := a.runner.RemoveScript(req.WorkloadId)
	if err != nil {
		return err
	}

	return nil
}

func (a *JavaScriptAgent) ListWorkloads() error {
	return nil
}

func (a *JavaScriptAgent) Trigger(workloadId string, payload []byte) ([]byte, error) {
	payload, err := a.runner.TriggerScript(workloadId, payload)
	if err != nil {
		return nil, err
	}

	return payload, nil
}
