package nexagent

import (
	"fmt"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

func LogError(msg string) {
	submitLog(msg, agentapi.LogLevelError)
}

func LogDebug(msg string) {
	submitLog(msg, agentapi.LogLevelDebug)
}

func LogInfo(msg string) {
	submitLog(msg, agentapi.LogLevelInfo)
}

func submitLog(msg string, lvl agentapi.LogLevel) {
	select {
	case agentLogs <- &agentapi.LogEntry{
		Source: "nex-agent",
		Level:  lvl,
		Text:   msg,
	}: // noop
	default: // noop
	}
}

func PublishWorkloadStarted(vmId string, workloadName string, totalBytes int32) {
	select {
	case agentLogs <- &agentapi.LogEntry{
		Source: "nex-agent",
		Level:  agentapi.LogLevelInfo,
		Text:   fmt.Sprintf("Workload %s started", workloadName),
	}: // noop
	default:
		// noop
	}

	evt := agentapi.NewAgentEvent(vmId, agentapi.WorkloadStartedEventType, agentapi.WorkloadStatusEvent{WorkloadName: workloadName})
	select {
	case eventLogs <- evt: // noop
	default:
		// noop
	}
}

func PublishWorkloadStopped(vmId string, workloadName string, err bool, message string) {
	level := agentapi.LogLevelInfo
	code := 0
	if err {
		level = agentapi.LogLevelError
		code = -1
	}
	select {
	case agentLogs <- &agentapi.LogEntry{
		Source: "nex-agent",
		Level:  agentapi.LogLevel(level),
		Text:   fmt.Sprintf("Workload %s stopped", workloadName),
	}: // noop
	default: // noop
	}

	evt := agentapi.NewAgentEvent(vmId, agentapi.WorkloadStoppedEventType, agentapi.WorkloadStatusEvent{WorkloadName: workloadName, Code: code, Message: message})
	select {
	case eventLogs <- evt: // noop
	default:
		// noop
	}
}
