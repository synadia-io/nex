package models

import (
	"fmt"
	"strings"
)

type Agent struct {
	Uri  string
	Argv []string
	Env  map[string]string

	// process *os.Process
}

type (
	AgentState string
)

const (
	DirectStartActorName = "direct-start"

	RunRequestKVBucket = "run_request"

	AgentStateStarting AgentState = "starting"
	AgentStateRunning  AgentState = "running"
	AgentStateStopping AgentState = "stopping"
	AgentStateLameduck AgentState = "lameduck"
	AgentStateError    AgentState = "error"
)

// $NEX.SVC.agentid.event.TYPE
func AgentAPIEmitEventSubject(inAgentId, eventType string) string {
	return fmt.Sprintf("%s.%s", EventAPIPrefix(inAgentId), strings.ToUpper(eventType))
}

// $NEX.SVC.namespace.logs.workloadid.{stdout|stderr}
func AgentEmitLogSubject(inNamespace, inWorkloadId string) string {
	return fmt.Sprintf("%s.%s.*", LogAPIPrefix(inNamespace), inWorkloadId)
}

// $NEX.SVC.nodeid.agent.LREGISTER.*
func AgentAPIRegisterSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf("%s.LREGISTER.*", AgentAPIPrefix(inNodeId))
}

// $NEX.SVC.nodeid.agent.LREGISTER.agentid
func AgentAPIRegisterRequestSubject(inAgentId, inNodeId string) string {
	return fmt.Sprintf("%s.LREGISTER.%s", AgentAPIPrefix(inNodeId), inAgentId)
}

// $NEX.SVC.*.agent.RREGISTER
func AgentAPIInitRemoteRegisterSubscribeSubject(nexus string) string {
	return fmt.Sprintf("%s.RREGISTER.*", AgentAPIPrefix(nexus))
}

// $NEX.SVC.signingKey.agent.RREGISTER
func AgentAPIInitRemoteRegisterRequestSubject(nexus, inSigningKey string) string {
	return fmt.Sprintf("%s.RREGISTER.%s", AgentAPIPrefix(nexus), inSigningKey)
}

// $NEX.SVC.nodeid.agent.HEARTBEAT.agentId
func AgentAPIHeartbeatSubject(inNodeId, inAgentId string) string {
	return fmt.Sprintf("%s.HEARTBEAT.%s", AgentAPIPrefix(inNodeId), inAgentId)
}

// $NEX.SVC.nodeid.agent.SETLAMEDUCK
func AgentAPISetLameduckSubject(inNodeId string) string {
	return fmt.Sprintf("%s.SETLAMEDUCK", AgentAPIPrefix(inNodeId))
}

// $NEX.SVC.nodeid.agent.agent_name.PING
func AgentAPIPingSubject(inNodeId, inAgentName string) string {
	return fmt.Sprintf("%s.%s.PING", AgentAPIPrefix(inNodeId), inAgentName)
}

// $NEX.SVC.nodeid.agent.PING
func AgentAPIPingAllSubject(inNodeId string) string {
	return fmt.Sprintf("%s.PING", AgentAPIPrefix(inNodeId))
}

// $NEX.SVC.nodeid.agent.QUERYWORKLOADS
func AgentAPIQueryWorkloadsSubject(inNodeId string) string {
	return fmt.Sprintf("%s.QUERYWORKLOADS", AgentAPIPrefix(inNodeId))
}

// $NEX.SVC.nodeid.agent.STARTWORKLOAD.agent_name
func AgentAPIStartWorkloadSubscribeSubject(inNodeId, inAgentName string) string {
	return fmt.Sprintf("%s.%s.STARTWORKLOAD.*", AgentAPIPrefix(inNodeId), inAgentName)
}

// $NEX.SVC.nodeid.agent.STARTWORKLOAD.*
func AgentAPIStopWorkloadSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf("%s.STOPWORKLOAD.*", AgentAPIPrefix(inNodeId))
}

// $NEX.SVC.nodeid.agent.GETWORKLOAD.*
func AgentAPIGetWorkloadSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf("%s.GETWORKLOAD.*", AgentAPIPrefix(inNodeId))
}

// $NEX.SVC.nexus.agent.PINGWORKLOAD.*
func AgentAPIPingWorkloadSubscribeSubject(inNexus string) string {
	return fmt.Sprintf("%s.PINGWORKLOAD.*", AgentAPIPrefix(inNexus))
}

// $NEX.SVC.nexus.agent.PINGWORKLOAD.workloadid
func AgentAPIPingWorkloadRequestSubject(inNexus, inWorkloadId string) string {
	return fmt.Sprintf("%s.PINGWORKLOAD.%s", AgentAPIPrefix(inNexus), inWorkloadId)
}

// $NEX.SVC.nodeid.agent.STARTWORKLOAD.agentid.workloadid
func AgentAPIStartWorkloadRequestSubject(inNodeId, inAgentId, inWorkloadId string) string {
	return fmt.Sprintf("%s.%s.STARTWORKLOAD.%s", AgentAPIPrefix(inNodeId), inAgentId, inWorkloadId)
}

// $NEX.SVC.nodeid.agent.STOPWORKLOAD.workloadid
func AgentAPIStopWorkloadRequestSubject(inNodeId, workloadId string) string {
	return fmt.Sprintf("%s.STOPWORKLOAD.%s", AgentAPIPrefix(inNodeId), workloadId)
}

// $NEX.SVC.nodeid.agent.GETWORKLOAD.workloadid
func AgentAPIGetWorkloadRequestSubject(inNodeId, workloadId string) string {
	return fmt.Sprintf("%s.GETWORKLOAD.%s", AgentAPIPrefix(inNodeId), workloadId)
}
