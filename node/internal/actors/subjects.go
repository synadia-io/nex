package actors

import (
	"fmt"

	"github.com/synadia-io/nex/models"
)

// Subscribe Subjects
func UndeploySubscribeSubject(inNodeId string) string {
	return fmt.Sprintf(models.APIPrefix+".UNDEPLOY.*.%s", inNodeId)
}

func DeploySubscribeSubject(inNodeId string) string {
	return fmt.Sprintf(models.APIPrefix+".DEPLOY.*.%s", inNodeId)
}

func AuctionDeploySubscribeSubject() string {
	return fmt.Sprintf(models.APIPrefix + ".ADEPLOY.*.*")
}

func InfoSubscribeSubject(inNodeId string) string {
	return fmt.Sprintf(models.APIPrefix+".INFO.*.%s", inNodeId)
}
