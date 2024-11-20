package models

import "fmt"

const (
	ControlAPIPrefix = "$NEX.control"
	LogAPIPrefix     = "$NEX.logs"
	EventAPIPrefix   = "$NEX.events"
)

// System only subjects
func PingSubject() string {
	return ControlAPIPrefix + ".system.PING"
}

func DirectDeploySubject(nodeId string) string {
	return fmt.Sprintf(ControlAPIPrefix+".system.DDEPLOY.%s", nodeId)
}

func LameduckSubject(inNodeId string) string {
	return fmt.Sprintf(ControlAPIPrefix+".system.LAMEDUCK.%s", inNodeId)
}

func DirectPingSubject(inNodeId string) string {
	return fmt.Sprintf(ControlAPIPrefix+".system.PING.%s", inNodeId)
}

// WPING subjects
func NamespacePingRequestSubject(inNS string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.WPING", inNS)
}

func WorkloadPingRequestSubject(inType, inNS, inWorkload string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.WPING.%s.%s", inNS, inType, inWorkload)
}

// Request subjects
func AuctionRequestSubject(inNS string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.AUCTION", inNS)
}

func AuctionDeployRequestSubject(inNS, inBidId string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.ADEPLOY.%s", inNS, inBidId)
}

func UndeployRequestSubject(inNS, inWorkloadID string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.UNDEPLOY.%s", inNS, inWorkloadID)
}

func InfoRequestSubject(inNS, inNodeId string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.INFO.%s", inNS, inNodeId)
}

func CloneWorkloadRequestSubject(inNS, inWorkloadID string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.CLONE.%s", inNS, inWorkloadID)
}
