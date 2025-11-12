package models

import "fmt"

const (
	NodeSystemNamespace = "system"
)

var (
	// Services
	ControlAPIPrefix = func(ns string) string { return fmt.Sprintf("$NEX.SVC.%s.control", ns) }
	AgentAPIPrefix   = func(ns string) string { return fmt.Sprintf("$NEX.SVC.%s.agent", ns) }

	// Feeds
	LogAPIPrefix     = func(ns string) string { return fmt.Sprintf("$NEX.FEED.%s.logs", ns) }
	MetricsAPIPrefix = func(ns string) string { return fmt.Sprintf("$NEX.FEED.%s.metrics", ns) }
	EventAPIPrefix   = func(ns string) string { return fmt.Sprintf("$NEX.FEED.%s.events", ns) }
)

// $NEX.SVC.namespace.control.PING
func PingRequestSubject(inNamespace string) string {
	return fmt.Sprintf("%s.PING", ControlAPIPrefix(inNamespace))
}

// $NEX.SVC.system.control.PING
func PingSubscribeSubject() string {
	return fmt.Sprintf("%s.PING", ControlAPIPrefix(NodeSystemNamespace))
}

// $NEX.SVC.system.control.HEARTBEAT.nodeid
func NodeEmitHeartbeatSubject(inNodeId string) string {
	return fmt.Sprintf("%s.HEARTBEAT.%s", ControlAPIPrefix(NodeSystemNamespace), inNodeId)
}

// $NEX.SVC.namespace.control.LAMEDUCK.nodeid
func LameduckRequestSubject(inNamespace, inNodeId string) string {
	return fmt.Sprintf("%s.LAMEDUCK.%s", ControlAPIPrefix(inNamespace), inNodeId)
}

// $NEX.SVC.system.control.LAMEDUCK.nodeid
func LameduckSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf("%s.LAMEDUCK.%s", ControlAPIPrefix(NodeSystemNamespace), inNodeId)
}

// $NEX.SVC.namespace.control.PING.nodeid
func DirectPingRequestSubject(inNamespace, inNodeId string) string {
	return fmt.Sprintf("%s.PING.%s", ControlAPIPrefix(inNamespace), inNodeId)
}

// $NEX.SVC.system.control.PING.nodeid
func DirectPingSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf("%s.PING.%s", ControlAPIPrefix(NodeSystemNamespace), inNodeId)
}

// $NEX.SVC.namespace.control.INFO.nodeid
func NodeInfoRequestSubject(inNamespace, inNodeId string) string {
	return fmt.Sprintf("%s.INFO.%s", ControlAPIPrefix(inNamespace), inNodeId)
}

// $NEX.SVC.system.control.INFO.nodeid
func NodeInfoSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf("%s.INFO.%s", ControlAPIPrefix(NodeSystemNamespace), inNodeId)
}

// $NEX.SVC.namespace.control.WPING
func NamespacePingRequestSubject(inNS string) string {
	return fmt.Sprintf("%s.WPING", ControlAPIPrefix(inNS))
}

// $NEX.SVC.namespace.control.WPING.workloadid
func WorkloadPingRequestSubject(inNS, inWorkload string) string {
	return fmt.Sprintf("%s.WPING.%s", ControlAPIPrefix(inNS), inWorkload)
}

// $NEX.SVC.namespace.control.AUCTION
func AuctionRequestSubject(inNS string) string {
	return fmt.Sprintf("%s.AUCTION", ControlAPIPrefix(inNS))
}

// $NEX.SVC.namespace.control.ADEPLOY.bidid
func AuctionDeployRequestSubject(inNS, inBidId string) string {
	return fmt.Sprintf("%s.ADEPLOY.%s", ControlAPIPrefix(inNS), inBidId)
}

// $NEX.SVC.namespace.control.UNDEPLOY.workloadid
func UndeployRequestSubject(inNS, inWorkloadID string) string {
	return fmt.Sprintf("%s.UNDEPLOY.%s", ControlAPIPrefix(inNS), inWorkloadID)
}

// $NEX.SVC.namespace.control.CLONE.workloadid
func CloneWorkloadRequestSubject(inNS, inWorkloadID string) string {
	return fmt.Sprintf("%s.CLONE.%s", ControlAPIPrefix(inNS), inWorkloadID)
}

// $NEX.SVC.system.control.AGENTID.nodeid
func GetAgentIdByNameSubject(inNodeId string) string {
	return fmt.Sprintf("%s.AGENTID.%s", ControlAPIPrefix(SystemNamespace), inNodeId)
}

// $NEX.SVC.*.control.AUCTION
func AuctionSubscribeSubject() string {
	return fmt.Sprintf("%s.AUCTION", ControlAPIPrefix("*"))
}

// $NEX.SVC.*.control.UNDEPLOY.workloadid
func UndeploySubscribeSubject() string {
	return fmt.Sprintf("%s.UNDEPLOY.*", ControlAPIPrefix("*"))
}

// $NEX.SVC.*.control.ADEPLOY.bidid
func AuctionDeploySubscribeSubject() string {
	return fmt.Sprintf("%s.ADEPLOY.*", ControlAPIPrefix("*"))
}

// $NEX.SVC.*.control.CLONE.workloadid
func CloneWorkloadSubscribeSubject() string {
	return fmt.Sprintf("%s.CLONE.*", ControlAPIPrefix("*"))
}

// $NEX.SVC.*.control.WPING
func NamespacePingSubscribeSubject() string {
	return fmt.Sprintf("%s.WPING", ControlAPIPrefix("*"))
}

// $NEX.SVC.*.control.WPING.workloadid
func WorkloadPingSubscribeSubject() string {
	return fmt.Sprintf("%s.WPING.*", ControlAPIPrefix("*"))
}
