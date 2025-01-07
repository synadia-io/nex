package models

import "fmt"

type AgentState string

const (
	DirectStartActorName = "direct-start"

	RunRequestKVBucket = "run_request"

	AgentStateRunning  AgentState = "running"
	AgentStateStopping AgentState = "stopping"
	AgentStateLameduck AgentState = "lameduck"
	AgentStateError    AgentState = "error"
)

// Subject map for internal comms between host and agents
// System only
func AgentAPIRegisterSubject() string {
	return fmt.Sprintf("%s.%s.REGISTER", AgentAPIPrefix, NodeSystemNamespace)
}

func AgentAPIHeartbeatSubject(inAgentName string) string {
	return fmt.Sprintf("%s.%s.%s.HEARTBEAT", AgentAPIPrefix, NodeSystemNamespace, inAgentName)
}

func AgentAPISetLameduckSubject(inAgentName string) string {
	return fmt.Sprintf("%s.%s.%s.SETLAMEDUCK", AgentAPIPrefix, NodeSystemNamespace, inAgentName)
}

func AgentAPIPingWorkloadSubject(inAgentName string) string {
	return fmt.Sprintf("%s.%s.%s.PINGWORKLOAD", AgentAPIPrefix, NodeSystemNamespace, inAgentName)
}

func AgentAPIPingSubject(inAgentName string) string {
	return fmt.Sprintf("%s.%s.%s.PING", AgentAPIPrefix, NodeSystemNamespace, inAgentName)
}

// User based
func AgentAPIStartWorkloadSubject(inNamespace, inAgentName string) string {
	return fmt.Sprintf("%s.%s.%s.STARTWORKLOAD.*", AgentAPIPrefix, inNamespace, inAgentName)
}

func AgentAPIStopWorkloadSubject(inNamespace, inAgentName string) string {
	return fmt.Sprintf("%s.%s.%s.STOPWORKLOAD", AgentAPIPrefix, inNamespace, inAgentName)
}

func AgentAPIQueryWorkloadSubject(inNamespace, inAgentName string) string {
	return fmt.Sprintf("%s.%s.%s.QUERYWORKLOAD", AgentAPIPrefix, inNamespace, inAgentName)
}
