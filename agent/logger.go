package nexagent

import (
	"fmt"
	"github.com/synadia-io/nex/internal/agentlogger"
	"os"

	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const NexEventSourceNexAgent = "nex-agent"

// logEmitter implements the writer interface that allows us to capture a workload's
// stdout and stderr so that we can then publish those logs to the host node
type logEmitter struct {
	name   string
	stderr bool

	logger *agentlogger.AgentLogger
}

// Write arbitrary bytes to the underlying log emitter
func (l *logEmitter) Write(bytes []byte) (int, error) {
	var lvl agentlogger.LogLevel
	if l.stderr {
		lvl = agentlogger.LogLevelError
	} else {
		lvl = agentlogger.LogLevelInfo
	}

	l.logger.LogEntry(&agentlogger.LogEntry{
		Level:  lvl,
		Source: l.name,
		Text:   string(bytes),
	})

	// FIXME-- this never returns an error
	return len(bytes), nil
}

func (a *Agent) LogDebug(msg string) {
	a.logger.LogMessage(agentlogger.LogLevelDebug, msg)
	fmt.Fprintln(os.Stdout, msg)
}

func (a *Agent) LogError(msg string) {
	a.logger.LogMessage(agentlogger.LogLevelError, msg)
	fmt.Fprintln(os.Stderr, msg)
}

func (a *Agent) LogInfo(msg string) {
	a.logger.LogMessage(agentlogger.LogLevelInfo, msg)
	fmt.Fprintln(os.Stdout, msg)
}

// FIXME-- revisit error handling
func (a *Agent) PublishWorkloadDeployed(vmID, workloadName string, totalBytes int64) {
	a.logger.LogEntry(&agentlogger.LogEntry{
		Source: NexEventSourceNexAgent,
		Level:  agentlogger.LogLevelInfo,
		Text:   fmt.Sprintf("Workload %s deployed", workloadName),
	})
	evt := agentapi.NewAgentEvent(vmID, agentapi.WorkloadStartedEventType, agentapi.WorkloadStatusEvent{WorkloadName: workloadName})
	a.eventLogs <- &evt
}

// PublishWorkloadExited publishes a workload failed or stopped message
// FIXME-- revisit error handling
func (a *Agent) PublishWorkloadExited(vmID, workloadName, message string, err bool, code int) {
	level := agentlogger.LogLevelInfo
	if err {
		level = agentlogger.LogLevelError
	}

	// FIXME-- this hack is here to get things working... refactor me
	txt := fmt.Sprintf("Workload %s exited", workloadName)
	if code == -1 {
		txt = fmt.Sprintf("Workload %s failed to deploy", workloadName)
	}

	a.logger.LogEntry(&agentlogger.LogEntry{
		Source: NexEventSourceNexAgent,
		Level:  agentlogger.LogLevel(level),
		Text:   txt,
	})

	evt := agentapi.NewAgentEvent(vmID, agentapi.WorkloadStoppedEventType, agentapi.WorkloadStatusEvent{WorkloadName: workloadName, Code: code, Message: message})
	a.eventLogs <- &evt
}
