package models

import "fmt"

const (
	ControlAPIPrefix = "$NEX.control"
	LogAPIPrefix     = "$NEX.logs"
	EventAPIPrefix   = "$NEX.events"
	AgentAPIPrefix   = "$NEX.agent"
)

// System only subjects
func PingSubject() string {
	return fmt.Sprintf("%s.%s.PING", ControlAPIPrefix, NodeSystemNamespace)
}

func DirectDeploySubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.DDEPLOY.%s", ControlAPIPrefix, NodeSystemNamespace, inNodeId)
}

func LameduckSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.LAMEDUCK.%s", ControlAPIPrefix, NodeSystemNamespace, inNodeId)
}

func DirectPingSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.PING.%s", ControlAPIPrefix, NodeSystemNamespace, inNodeId)
}

func InfoSubject(inNodeId string) string {
	return fmt.Sprintf("%s.%s.INFO.%s", ControlAPIPrefix, NodeSystemNamespace, inNodeId)
}

// WPING subjects
func NamespacePingRequestSubject(inNS string) string {
	return fmt.Sprintf("%s.%s.WPING", ControlAPIPrefix, inNS)
}

func WorkloadPingRequestSubject(inNS, inWorkload string) string {
	return fmt.Sprintf("%s.%s.WPING.%s", ControlAPIPrefix, inNS, inWorkload)
}

// Request subjects
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
