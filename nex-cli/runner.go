package nexcli

import (
	"fmt"
	"os"

	controlapi "github.com/ConnectEverything/nex/control-api"
	"github.com/choria-io/fisk"
	"github.com/nats-io/nkeys"
)

func RunWorkload(ctx *fisk.ParseContext) error {
	fmt.Printf("%v\n", Opts)
	fmt.Printf("%v\n", RunOpts)
	nc, err := generateConnectionFromOpts()
	if err != nil {
		return err
	}
	nodeClient := controlapi.NewApiClient(nc, Opts.Timeout)

	// Get node info so we can get public xkey from the target for env encryption
	nodeInfo, err := nodeClient.NodeInfo(RunOpts.TargetNode)
	if err != nil {
		fmt.Println("SUCK")
		return err
	}

	targetPublicXkey := nodeInfo.PublicXKey
	fmt.Println(targetPublicXkey)

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
		controlapi.TargetPublicXKey(RunOpts.TargetNode),
		controlapi.WorkloadName(RunOpts.Name),
		controlapi.WorkloadDescription(RunOpts.Description),
	)
	if err != nil {
		return nil
	}

	resp, err := nodeClient.StartWorkload(request)
	if err != nil {
		return err
	}

	renderRunResponse(resp)
	return nil
}

func renderRunResponse(resp *controlapi.RunResponse) {
	fmt.Printf("%v", resp)
}
