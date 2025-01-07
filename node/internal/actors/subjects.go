package actors

import (
	"fmt"

	"github.com/synadia-io/nex/models"
)

// Subscribe Subjects
func AuctionSubscribeSubject() string {
	// $NEX.control.namespace.AUCTION
	return models.ControlAPIPrefix + ".*.AUCTION"
}

func UndeploySubscribeSubject() string {
	// $NEX.control.namespace.UNDEPLOY.workloadid
	return models.ControlAPIPrefix + ".*.UNDEPLOY.*"
}

func AuctionDeploySubscribeSubject() string {
	// $NEX.control.namespace.ADEPLOY.bidid
	return models.ControlAPIPrefix + ".*.ADEPLOY.*"
}

func CloneWorkloadSubscribeSubject() string {
	// $NEX.control.namespace.CLONE.workloadid
	return models.ControlAPIPrefix + ".*.CLONE.*"
}

func NamespacePingSubscribeSubject() string {
	// $NEX.control.namespace.WPING
	return models.ControlAPIPrefix + ".*.WPING"
}

func WorkloadPingSubscribeSubject() string {
	// $NEX.control.namespace.WPING.workloadid
	return models.ControlAPIPrefix + ".*.WPING.*"
}

// Internal NATS server agent REQUEST subjects
func AgentAPIStartWorkloadRequestSubject(inNamespace, inAgentName, inWorkloadId string) string {
	return fmt.Sprintf("%s.%s.%s.STARTWORKLOAD.%s", models.AgentAPIPrefix, inNamespace, inAgentName, inWorkloadId)
}
