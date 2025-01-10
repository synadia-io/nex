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

type AgentState string

const (
	DirectStartActorName = "direct-start"

	RunRequestKVBucket = "run_request"

	AgentStateStarting AgentState = "starting"
	AgentStateRunning  AgentState = "running"
	AgentStateStopping AgentState = "stopping"
	AgentStateLameduck AgentState = "lameduck"
	AgentStateError    AgentState = "error"
)

// Subject map for internal comms between host and agents
// System only
func AgentAPILocalRegisterSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf("%s.*.REGISTER.%s", AgentAPIPrefix, inNodeId)
}

func AgentAPILocalRegisterRequestSubject(inAgentId, inNodeId string) string {
	return fmt.Sprintf("%s.%s.REGISTER.%s", AgentAPIPrefix, inAgentId, inNodeId)
}

// Deprecated
func AgentAPILocalRegisterSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.REGISTER.%s", AgentAPIPrefix, NodeSystemNamespace, inNodeId)
}

func AgentAPIRemoteRegisterSubject() string {
	return fmt.Sprintf("%s.%s.REGISTER", AgentAPIPrefix, NodeSystemNamespace)
}

func AgentAPIHeartbeatSubject(inAgentName string) string {
	return fmt.Sprintf("%s.%s.HEARTBEAT", AgentAPIPrefix, inAgentName)
}

func AgentAPIEmitEventSubject(inAgentId, eventType string) string {
	return fmt.Sprintf("%s.%s.%s", EventAPIPrefix, inAgentId, strings.ToUpper(eventType))
}

func AgentAPISetLameduckSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.SETLAMEDUCK", AgentAPIPrefix, inNodeId)
}

func AgentAPIPingSubject(inNodeId, inAgentName string) string {
	return fmt.Sprintf("%s.%s.%s.PING", AgentAPIPrefix, inNodeId, inAgentName)
}

func AgentAPIPingAllSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.PING", AgentAPIPrefix, inNodeId)
}

// AgentAPI Pub/Sub Subjects
func AgentAPIQueryWorkloadsSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.QUERYWORKLOADS", AgentAPIPrefix, inNodeId)
}

// AgentAPI Subscribe subjects
func AgentAPIStartWorkloadSubscribeSubject(inNodeId, inAgentName string) string {
	return fmt.Sprintf("%s.%s.%s.STARTWORKLOAD.*", AgentAPIPrefix, inNodeId, inAgentName)
}

func AgentAPIStopWorkloadSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.STOPWORKLOAD.*", AgentAPIPrefix, inNodeId)
}

func AgentAPIGetWorkloadSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.GETWORKLOAD.*", AgentAPIPrefix, inNodeId)
}

func AgentAPIPingWorkloadSubscribeSubject() string {
	return fmt.Sprintf("%s.PINGWORKLOAD.*", AgentAPIPrefix)
}

func AgentAPIPingWorkloadRequestSubject(inWorkloadId string) string {
	return fmt.Sprintf("%s.PINGWORKLOAD.%s", AgentAPIPrefix, inWorkloadId)
}

// AgentAPI Request subjects
func AgentAPIStartWorkloadRequestSubject(inNodeId, inAgentId, inWorkloadId string) string {
	return fmt.Sprintf("%s.%s.%s.STARTWORKLOAD.%s", AgentAPIPrefix, inNodeId, inAgentId, inWorkloadId)
}

func AgentAPIStopWorkloadRequestSubject(inNodeId, workloadId string) string {
	return fmt.Sprintf("%s.%s.STOPWORKLOAD.%s", AgentAPIPrefix, inNodeId, workloadId)
}

func AgentAPIGetWorkloadRequestSubject(inNodeId, workloadId string) string {
	return fmt.Sprintf("%s.%s.GETWORKLOAD.%s", AgentAPIPrefix, inNodeId, workloadId)
}

// Node Request subjects
func PingAgentsRequestSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.*.PING", AgentAPIPrefix, inNodeId)
}

// Agent log emit endpoints
func AgentEmitLogSubject(inNamespace, inWorkloadId string) string {
	return fmt.Sprintf("%s.%s.%s", LogAPIPrefix, inNamespace, inWorkloadId)
}
