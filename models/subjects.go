package models

import "fmt"

const (
	ControlAPIPrefix = "$NEX.control"
	LogAPIPrefix     = "$NEX.logs"
	EventAPIPrefix   = "$NEX.events"
)

// System only subjects
func PingSubject() string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.PING", NodeSystemNamespace)
}

func DirectDeploySubject(inNodeId string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.DDEPLOY.%s", NodeSystemNamespace, inNodeId)
}

func LameduckSubject(inNodeId string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.LAMEDUCK.%s", NodeSystemNamespace, inNodeId)
}

func DirectPingSubject(inNodeId string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.PING.%s", NodeSystemNamespace, inNodeId)
}

func InfoSubject(inNodeId string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.INFO.%s", NodeSystemNamespace, inNodeId)
}

// WPING subjects
func NamespacePingRequestSubject(inNS string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.WPING", inNS)
}

func WorkloadPingRequestSubject(inNS, inWorkload string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.WPING.%s", inNS, inWorkload)
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

func CloneWorkloadRequestSubject(inNS, inWorkloadID string) string {
	return fmt.Sprintf(ControlAPIPrefix+".%s.CLONE.%s", inNS, inWorkloadID)
}
