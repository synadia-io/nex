package nexagent

import (
	"fmt"
	"os"

	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const NexEventSourceNexAgent = "nex-agent"

// logEmitter implements the writer interface that allows us to capture a workload's
// stdout and stderr so that we can then publish those logs to the host node
type logEmitter struct {
	name   string
	stderr bool

	logs chan *agentapi.LogEntry
}

// Write arbitrary bytes to the underlying log emitter
func (l *logEmitter) Write(bytes []byte) (int, error) {
	var lvl agentapi.LogLevel
	if l.stderr {
		lvl = agentapi.LogLevelError
	} else {
		lvl = agentapi.LogLevelInfo
	}

	l.logs <- &agentapi.LogEntry{
		Level:  lvl,
		Source: l.name,
		Text:   string(bytes),
	}

	// FIXME-- this never returns an error
	return len(bytes), nil
}

func (a *Agent) LogDebug(msg string) {
	fmt.Fprintln(os.Stdout, msg)
	if a.sandboxed {
		a.submitLog(msg, agentapi.LogLevelDebug)
	}
}

func (a *Agent) LogError(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	if a.sandboxed {
		a.submitLog(msg, agentapi.LogLevelError)
	}
}

func (a *Agent) LogInfo(msg string) {
	fmt.Fprintln(os.Stdout, msg)
	if a.sandboxed {
		a.submitLog(msg, agentapi.LogLevelInfo)
	}
}

// FIXME-- revisit error handling
func (a *Agent) PublishWorkloadDeployed(vmID, workloadName string, totalBytes int64) {
	a.agentLogs <- &agentapi.LogEntry{
		Source: NexEventSourceNexAgent,
		Level:  agentapi.LogLevelInfo,
		Text:   fmt.Sprintf("Workload %s deployed", workloadName),
	}

	evt := agentapi.NewAgentEvent(vmID, agentapi.WorkloadDeployedEventType, agentapi.WorkloadStatusEvent{WorkloadName: workloadName})
	a.eventLogs <- &evt
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

	a.agentLogs <- &agentapi.LogEntry{
		Source: NexEventSourceNexAgent,
		Level:  agentapi.LogLevel(level),
		Text:   txt,
	}

	evt := agentapi.NewAgentEvent(vmID, agentapi.WorkloadUndeployedEventType, agentapi.WorkloadStatusEvent{WorkloadName: workloadName, Code: code, Message: message})
	a.eventLogs <- &evt
}
