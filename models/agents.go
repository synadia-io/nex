package models

import "fmt"

const (
	DirectStartActorName = "direct-start"

	RunRequestKVBucket = "run_request"
)

// Subject map for internal comms between host and agents
func AgentAPIRegisterSubject() string {
	return fmt.Sprintf("%s.%s.REGISTER", AgentAPIPrefix, NodeSystemNamespace)
}

func AgentAPIHeartbeatSubject(inAgentName string) string {
	return fmt.Sprintf("%s.%s.%s.HEARTBEAT", AgentAPIPrefix, NodeSystemNamespace, inAgentName)
}

func AgentAPIStartWorkloadSubscribeSubject(inAgentName string) string {
	return fmt.Sprintf("%s.%s.%s.STARTWORKLOAD.*", AgentAPIPrefix, NodeSystemNamespace, inAgentName)
}

func AgentAPIStartWorkloadRequestSubject(inAgentName, inWorkloadId string) string {
	return fmt.Sprintf("%s.%s.%s.STARTWORKLOAD.%s", AgentAPIPrefix, NodeSystemNamespace, inAgentName, inWorkloadId)
}

func AgentAPIStopWorkloadSubject(inAgentName string) string {
	return fmt.Sprintf("%s.%s.%s.STOPWORKLOAD", AgentAPIPrefix, NodeSystemNamespace, inAgentName)
}

func AgentAPIQueryWorkloadSubject(inAgentName string) string {
	return fmt.Sprintf("%s.%s.%s.QUERYWORKLOAD", AgentAPIPrefix, NodeSystemNamespace, inAgentName)
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
