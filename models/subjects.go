package models

import "fmt"

const (
	NodeSystemNamespace = "system"

	ControlAPIPrefix  = "$NEX.control"
	LogAPIPrefix      = "$NEX.logs"
	EventAPIPrefix    = "$NEX.events"
	AgentAPIPrefix    = "$NEX.agent"
	WorkloadAPIPrefix = "$NEX.workload"
)

// System only agent subjects
func StartAgentSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.START.%s", AgentAPIPrefix, NodeSystemNamespace, inNodeId)
}

func StopAgentSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.STOP.%s", AgentAPIPrefix, NodeSystemNamespace, inNodeId)
}

func RegisterRemoteAgentSubject() string {
	return fmt.Sprintf("%s.%s.REGISTER", AgentAPIPrefix, NodeSystemNamespace)
}

func RegisterLocalAgentSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.REGISTER.%s", AgentAPIPrefix, NodeSystemNamespace, inNodeId)
}

func EmitSystemEventSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.%s", EventAPIPrefix, NodeSystemNamespace, inNodeId)
}

// System only subjects
func PingRequestSubject(inNamespace string) string {
	return fmt.Sprintf("%s.%s.PING", ControlAPIPrefix, inNamespace)
}

func PingSubscribeSubject() string {
	return fmt.Sprintf("%s.%s.PING", ControlAPIPrefix, NodeSystemNamespace)
}

func NodeEmitHeartbeatSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.HEARTBEAT.%s", ControlAPIPrefix, NodeSystemNamespace, inNodeId)
}

func DirectDeploySubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.DDEPLOY.%s", ControlAPIPrefix, NodeSystemNamespace, inNodeId)
}

func LameduckRequestSubject(inNamespace, inNodeId string) string {
	return fmt.Sprintf("%s.%s.LAMEDUCK.%s", ControlAPIPrefix, inNamespace, inNodeId)
}

func LameduckSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.LAMEDUCK.%s", ControlAPIPrefix, NodeSystemNamespace, inNodeId)
}

func DirectPingRequestSubject(inNamespace, inNodeId string) string {
	return fmt.Sprintf("%s.%s.PING.%s", ControlAPIPrefix, inNamespace, inNodeId)
}

func DirectPingSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.PING.%s", ControlAPIPrefix, NodeSystemNamespace, inNodeId)
}

func NodeInfoRequestSubject(inNamespace, inNodeId string) string {
	return fmt.Sprintf("%s.%s.INFO.%s", ControlAPIPrefix, inNamespace, inNodeId)
}

func NodeInfoSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.INFO.%s", ControlAPIPrefix, NodeSystemNamespace, inNodeId)
}

func NamespacePingRequestSubject(inNS string) string {
	return fmt.Sprintf("%s.%s.WPING", ControlAPIPrefix, inNS)
}

func WorkloadPingRequestSubject(inNS, inWorkload string) string {
	return fmt.Sprintf("%s.%s.WPING.%s", ControlAPIPrefix, inNS, inWorkload)
}

func AuctionRequestSubject(inNS string) string {
	return fmt.Sprintf("%s.%s.AUCTION", ControlAPIPrefix, inNS)
}

func AuctionDeployRequestSubject(inNS, inBidId string) string {
	return fmt.Sprintf("%s.%s.ADEPLOY.%s", ControlAPIPrefix, inNS, inBidId)
}

func UndeployRequestSubject(inNS, inWorkloadID string) string {
	return fmt.Sprintf("%s.%s.UNDEPLOY.%s", ControlAPIPrefix, inNS, inWorkloadID)
}

func CloneWorkloadRequestSubject(inNS, inWorkloadID string) string {
	return fmt.Sprintf("%s.%s.CLONE.%s", ControlAPIPrefix, inNS, inWorkloadID)
}

// Subscribe Subjects
func AuctionSubscribeSubject() string {
	// $NEX.control.namespace.AUCTION
	return ControlAPIPrefix + ".*.AUCTION"
}

func UndeploySubscribeSubject() string {
	// $NEX.control.namespace.UNDEPLOY.workloadid
	return ControlAPIPrefix + ".*.UNDEPLOY.*"
}

func AuctionDeploySubscribeSubject() string {
	// $NEX.control.namespace.ADEPLOY.bidid
	return ControlAPIPrefix + ".*.ADEPLOY.*"
}

func CloneWorkloadSubscribeSubject() string {
	// $NEX.control.namespace.CLONE.workloadid
	return ControlAPIPrefix + ".*.CLONE.*"
}

func NamespacePingSubscribeSubject() string {
	// $NEX.control.namespace.WPING
	return ControlAPIPrefix + ".*.WPING"
}

func WorkloadPingSubscribeSubject() string {
	// $NEX.control.namespace.WPING.workloadid
	return ControlAPIPrefix + ".*.WPING.*"
}
