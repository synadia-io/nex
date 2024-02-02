package nexagent

import (
	"fmt"
	"os"

	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const NexEventSourceNexAgent = "nex-agent"

func (a *Agent) LogError(msg string) {
	a.submitLog(msg, agentapi.LogLevelError)
	fmt.Fprintln(os.Stderr, msg)
}

func (a *Agent) LogDebug(msg string) {
	a.submitLog(msg, agentapi.LogLevelDebug)
	fmt.Fprintln(os.Stdout, msg)
}

func (a *Agent) LogInfo(msg string) {
	a.submitLog(msg, agentapi.LogLevelInfo)
	fmt.Fprintln(os.Stdout, msg)
}

func (a *Agent) submitLog(msg string, lvl agentapi.LogLevel) {
	select {
	case a.agentLogs <- &agentapi.LogEntry{
		Source: NexEventSourceNexAgent,
		Level:  lvl,
		Text:   msg,
	}: // noop
	default: // noop
	}
}

// FIXME-- revisit error handling
func (a *Agent) PublishWorkloadDeployed(vmID, workloadName string, totalBytes int64) {
	select {
	case a.agentLogs <- &agentapi.LogEntry{
		Source: NexEventSourceNexAgent,
		Level:  agentapi.LogLevelInfo,
		Text:   fmt.Sprintf("Workload %s deployed", workloadName),
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

// PublishWorkloadExited publishes a workload failed or stopped message
// FIXME-- revisit error handling
func (a *Agent) PublishWorkloadExited(vmID, workloadName, message string, err bool, code int) {
	level := agentapi.LogLevelInfo
	if err {
		level = agentapi.LogLevelError
	}

	// FIXME-- this hack is here to get things working... refactor me
	txt := fmt.Sprintf("Workload %s exited", workloadName)
	if code == -1 {
		txt = fmt.Sprintf("Workload %s failed to deploy", workloadName)
	}

	select {
	case a.agentLogs <- &agentapi.LogEntry{
		Source: NexEventSourceNexAgent,
		Level:  agentapi.LogLevel(level),
		Text:   txt,
	}: // noop
	default: // noop
	}

	evt := agentapi.NewAgentEvent(vmID, agentapi.WorkloadStoppedEventType, agentapi.WorkloadStatusEvent{WorkloadName: workloadName, Code: code, Message: message})
	select {
	case a.eventLogs <- &evt: // noop
	default:
		// noop
	}
}
