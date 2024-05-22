package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/nats-io/nkeys"
	controlapi "github.com/synadia-io/nex/control-api"
	"github.com/synadia-io/nex/internal/models"
)

// Issues a request to stop a running workload
func StopWorkload(ctx context.Context, logger *slog.Logger) error {
	nc, err := models.GenerateConnectionFromOpts(Opts, logger)
	if err != nil {
		return err
	}

	nodeClient := controlapi.NewApiClientWithNamespace(nc, Opts.Timeout, Opts.Namespace, logger)

	issuerSeed, err := os.ReadFile(StopOpts.ClaimsIssuerFile)
	if err != nil {
		return err
	}

	issuerKp, err := nkeys.FromSeed(issuerSeed)
	if err != nil {
		return err
	}
	stopRequest, err := controlapi.NewStopRequest(StopOpts.WorkloadId, StopOpts.WorkloadName, StopOpts.TargetNode, issuerKp)
	if err != nil {
		fmt.Printf("⛔ Failed to create workload request: %s\n", err)
		return err
	}
	resp, err := nodeClient.StopWorkload(stopRequest)
	if err != nil {
		fmt.Printf("⛔ Workload stop request failed: %s\n", err)
		return err
	}

	renderStopResponse(resp)
	return nil
}

// Submits a run request for the given workload to the specified node
func RunWorkload(ctx context.Context, logger *slog.Logger) error {
	nc, err := models.GenerateConnectionFromOpts(Opts, logger)
	if err != nil {
		return err
	}
	nodeClient := controlapi.NewApiClientWithNamespace(nc, Opts.Timeout, Opts.Namespace, logger)

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

	if RunOpts.WorkloadType == "v8" && len(RunOpts.TriggerSubjects) == 0 {
		return errors.New("cannot start a function-type workload without specifying at least one trigger subject")
	}

	argv := []string{}
	if len(RunOpts.Argv) > 0 {
		argv = strings.Split(RunOpts.Argv, " ")
	}

	request, err := controlapi.NewDeployRequest(
		controlapi.Argv(argv),
		controlapi.Location(RunOpts.WorkloadUrl.String()),
		controlapi.Environment(RunOpts.Env),
		controlapi.Essential(RunOpts.Essential),
		controlapi.Issuer(issuerKp),
		controlapi.SenderXKey(xkey),
		controlapi.TargetNode(RunOpts.TargetNode),
		controlapi.TargetPublicXKey(targetPublicXkey),
		controlapi.WorkloadName(RunOpts.Name),
		controlapi.WorkloadType(RunOpts.WorkloadType),
		controlapi.TriggerSubjects(RunOpts.TriggerSubjects),
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

	renderRunResponse(RunOpts.TargetNode, resp)
	return nil
}

func renderRunResponse(targetNode string, resp *controlapi.RunResponse) {
	if resp.Started {
		fmt.Printf("🚀 Workload '%s' accepted. You can now refer to this workload with ID: %s on node %s", resp.Name, resp.ID, targetNode)
	} else {
		fmt.Println("⛔ Workload rejected")
	}
}

func renderStopResponse(resp *controlapi.StopResponse) {
	if resp.Stopped {
		fmt.Printf("✅ Workload '%s' stopped.\n", resp.Name)
	} else {
		fmt.Println("⛔ Workload failed to stop")
	}
}
