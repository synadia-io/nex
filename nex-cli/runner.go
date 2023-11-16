package nexcli

import (
	"fmt"
	"os"

	controlapi "github.com/ConnectEverything/nex/control-api"
	"github.com/choria-io/fisk"
	"github.com/nats-io/nkeys"
)

// Submits a run request for the given workload to the specified node
func RunWorkload(ctx *fisk.ParseContext) error {
	nc, err := generateConnectionFromOpts()
	if err != nil {
		return err
	}
	nodeClient := controlapi.NewApiClient(nc, Opts.Timeout)

	// Get node info so we can get public xkey from the target for env encryption
	nodeInfo, err := nodeClient.NodeInfo(RunOpts.TargetNode)
	if err != nil {
		return err
	}

	targetPublicXkey := nodeInfo.PublicXKey

	issuerSeed, err := os.ReadFile(RunOpts.ClaimsIssuerFile)
	if err != nil {
		return err
	}

	issuerKp, err := nkeys.FromSeed(issuerSeed)
	if err != nil {
		return err
	}
	xkeyRaw, err := os.ReadFile(RunOpts.PublisherXkeyFile)
	if err != nil {
		return nil
	}
	xkey, err := nkeys.FromCurveSeed(xkeyRaw)
	if err != nil {
		return err
	}

	request, err := controlapi.NewRunRequest(
		controlapi.Location(RunOpts.WorkloadUrl.String()),
		controlapi.Environment(RunOpts.Env),
		controlapi.Issuer(issuerKp),
		controlapi.SenderXKey(xkey),
		controlapi.TargetNode(RunOpts.TargetNode),
		controlapi.TargetPublicXKey(targetPublicXkey),
		controlapi.WorkloadName(RunOpts.Name),
		controlapi.Checksum("abc12345TODOmakethisreal"),
		controlapi.WorkloadDescription(RunOpts.Description),
	)
	if err != nil {
		return nil
	}

	resp, err := nodeClient.StartWorkload(request)
	if err != nil {
		fmt.Printf("⛔ Workload run request failed to submit: %s\n", err)
		return err
	}

	renderRunResponse(resp)
	return nil
}

func renderRunResponse(resp *controlapi.RunResponse) {
	if resp.Started {
		fmt.Printf("🚀 Workload '%s' accepted. You can now refer to this workload with ID: %s\n", resp.Name, resp.MachineId)
	} else {
		fmt.Println("⛔ Workload rejected")
	}
}
