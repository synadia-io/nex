package nexagent

import (
	"fmt"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

func (a *Agent) LogError(msg string) {
	a.submitLog(msg, agentapi.LogLevelError)
}

func (a *Agent) LogDebug(msg string) {
	a.submitLog(msg, agentapi.LogLevelDebug)
}

func (a *Agent) LogInfo(msg string) {
	a.submitLog(msg, agentapi.LogLevelInfo)
}

func (a *Agent) submitLog(msg string, lvl agentapi.LogLevel) {
	select {
	case a.agentLogs <- &agentapi.LogEntry{
		Source: "nex-agent",
		Level:  lvl,
		Text:   msg,
	}: // noop
	default: // noop
	}
}

func (a *Agent) PublishWorkloadStarted(vmID string, workloadName string, totalBytes int32) {
	select {
	case a.agentLogs <- &agentapi.LogEntry{
		Source: "nex-agent",
		Level:  agentapi.LogLevelInfo,
		Text:   fmt.Sprintf("Workload %s started", workloadName),
	}: // noop
	default:
		// noop
	}

	evt := agentapi.NewAgentEvent(vmID, agentapi.WorkloadStartedEventType, agentapi.WorkloadStatusEvent{WorkloadName: workloadName})
	select {
	case a.eventLogs <- &evt: // noop
	default:
		// noop
	}
}

// PublishWorkloadStopped publishes a workload stopped message
func (a *Agent) PublishWorkloadStopped(vmId string, workloadName string, err bool, message string) {
	level := agentapi.LogLevelInfo
	code := 0
	if err {
		level = agentapi.LogLevelError
		code = -1
	}
	select {
	case a.agentLogs <- &agentapi.LogEntry{
		Source: "nex-agent",
		Level:  agentapi.LogLevel(level),
		Text:   fmt.Sprintf("Workload %s stopped", workloadName),
	}: // noop
	default: // noop
	}

	evt := agentapi.NewAgentEvent(vmId, agentapi.WorkloadStoppedEventType, agentapi.WorkloadStatusEvent{WorkloadName: workloadName, Code: code, Message: message})
	select {
	case a.eventLogs <- &evt: // noop
	default:
		// noop
	}
}
