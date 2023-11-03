package nexagent

import (
	"fmt"
	"time"

	agentapi "github.com/ConnectEverything/nex/agent-api"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func LogError(msg string) {
	submitLog(msg, agentapi.LogLevel_LEVEL_ERROR)
}

func LogDebug(msg string) {
	submitLog(msg, agentapi.LogLevel_LEVEL_DEBUG)
}

func LogInfo(msg string) {
	submitLog(msg, agentapi.LogLevel_LEVEL_INFO)
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

func PublishAgentStarted() {
	select {
	case agentLogs <- &agentapi.LogEntry{
		Source: "nex-agent",
		Level:  agentapi.LogLevel_LEVEL_INFO,
		Text:   "Agent started",
	}:
		// NO OP
	default:
		// ALSO SKIP
	}

	select {
	case eventLogs <- &agentapi.AgentEvent{
		EventTime: now(),
		Data: &agentapi.AgentEvent_AgentStarted{AgentStarted: &agentapi.AgentStartedEvent{
			AgentVersion: VERSION,
		}},
	}: // noop
	default:
		// noop
	}
}

func PublishWorkloadStarted(workloadName string, totalBytes int32) {
	select {
	case agentLogs <- &agentapi.LogEntry{
		Source: "nex-agent",
		Level:  agentapi.LogLevel_LEVEL_INFO,
		Text:   fmt.Sprintf("Workload %s started", workloadName),
	}: // noop
	default:
		// noop
	}

	select {
	case eventLogs <- &agentapi.AgentEvent{
		EventTime: now(),
		Data: &agentapi.AgentEvent_WorkloadStarted{WorkloadStarted: &agentapi.WorkloadStartedEvent{
			Name:       workloadName,
			TotalBytes: totalBytes,
		}},
	}: // noop
	default:
		// noop
	}
}

func PublishWorkloadStopped(workloadName string, err bool, message string) {
	level := agentapi.LogLevel_LEVEL_INFO
	code := int32(0)
	if err {
		level = agentapi.LogLevel_LEVEL_ERROR
		code = int32(-1)
	}
	select {
	case agentLogs <- &agentapi.LogEntry{
		Source: "nex-agent",
		Level:  level,
		Text:   fmt.Sprintf("Workload %s stopped", workloadName),
	}: // noop
	default: // noop
	}

	select {
	case eventLogs <- &agentapi.AgentEvent{
		EventTime: now(),
		Data: &agentapi.AgentEvent_WorkloadStopped{WorkloadStopped: &agentapi.WorkloadStoppedEvent{
			Name:    workloadName,
			Code:    code,
			Message: message,
		}},
	}: // noop
	default: // noop
	}
}

func PublishAgentStopped() {
	select {
	case agentLogs <- &agentapi.LogEntry{
		Source: "agent",
		Level:  agentapi.LogLevel_LEVEL_INFO,
		Text:   "Agent stopped",
	}: // noop
	default: // noop
	}

	select {
	case eventLogs <- &agentapi.AgentEvent{
		EventTime: now(),
		Data:      &agentapi.AgentEvent_AgentStopped{AgentStopped: &agentapi.AgentStoppedEvent{}},
	}: // noop
	default: // noop
	}
}

func now() *timestamppb.Timestamp {
	return &timestamppb.Timestamp{Seconds: time.Now().UTC().Unix()}
}
