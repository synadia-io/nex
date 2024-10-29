package actors

import "fmt"

const (
	APIPrefix = "$NEX"
)

// Shared subscribe/request subjects
func AuctionSubject() string {
	return APIPrefix + ".AUCTION"
}

func PingSubject() string {
	return APIPrefix + ".PING"
}

func AgentPingSubscribeSubject() string {
	return APIPrefix + ".APING.>"
}

func LameduckSubject(inNodeId string) string {
	return fmt.Sprintf(APIPrefix+".LAMEDUCK.%s", inNodeId)
}

func DirectPingSubject(inNodeId string) string {
	return fmt.Sprintf(APIPrefix+".PING.%s", inNodeId)
}

// Subscribe Subjects
func UndeploySubscribeSubject(inNodeId string) string {
	return fmt.Sprintf(APIPrefix+".UNDEPLOY.*.%s", inNodeId)
}

func DeploySubscribeSubject(inNodeId string) string {
	return fmt.Sprintf(APIPrefix+".DEPLOY.*.%s", inNodeId)
}

func InfoSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf(APIPrefix+".INFO.*.%s", inNodeId)
}

// Request subjects
func AgentPingNamespaceRequestSubject(inNS string) string {
	return fmt.Sprintf(APIPrefix+".APING.%s", inNS)
}

func AgentPingWorkloadRequestSubject(inNS, inWorkload string) string {
	return fmt.Sprintf(APIPrefix+".APING.%s.%s", inNS, inWorkload)
}

func DeployRequestSubject(inNS, inNodeId string) string {
	return fmt.Sprintf(APIPrefix+".DEPLOY.%s.%s", inNS, inNodeId)
}

func UndeployRequestSubject(inNodeId string) string {
	return fmt.Sprintf(APIPrefix+".UNDEPLOY.*.%s", inNodeId)
}

func InfoRequestSubject(inNS, inNodeId string) string {
	return fmt.Sprintf(APIPrefix+".INFO.%s.%s", inNS, inNodeId)
}
