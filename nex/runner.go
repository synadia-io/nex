package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

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

	stopRequest, err := controlapi.NewStopRequest(StopOpts.TargetNode, StopOpts.WorkloadId, issuerKp)
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

	nodeClient := controlapi.NewApiClientWithNamespace(nc, time.Millisecond*25000, Opts.Namespace, logger)

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

	js, err := nc.JetStream()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed resolve jetstream: %s", err)
		return err
	}

	bucket, err := js.ObjectStore(RunOpts.WorkloadURL.Hostname())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed resolve workload object store: %s", err)
		return err
	}

	artifact := RunOpts.WorkloadURL.Path[1:len(RunOpts.WorkloadURL.Path)]
	fmt.Fprintf(os.Stdout, "resolved artifact name from workload url: %s", artifact)

	info, err := bucket.GetInfo(artifact)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed resolve workload artifact: %s; %s", artifact, err)
		return err
	}

	argv := []string{}
	if len(RunOpts.Argv) > 0 {
		argv = strings.Split(RunOpts.Argv, " ")
	}

	var hsConfig *controlapi.NatsJwtConnectionInfo
	if RunOpts.HsUrl != "" {
		hsConfig = &controlapi.NatsJwtConnectionInfo{
			NatsUrl:      RunOpts.HsUrl,
			NatsUserSeed: RunOpts.HsUserSeed,
			NatsUserJwt:  RunOpts.HsUserJwt,
		}
	}

	request, err := controlapi.NewDeployRequest(
		controlapi.Argv(argv),
		controlapi.Hash(controlapi.SanitizeNATSDigest(info.Digest)),
		controlapi.HostServicesConfig(hsConfig),
		controlapi.Environment(RunOpts.Env),
		controlapi.Essential(RunOpts.Essential),
		controlapi.Issuer(issuerKp),
		controlapi.JsDomain(Opts.JsDomain),
		controlapi.Location(RunOpts.WorkloadURL.String()),
		controlapi.SenderXKey(xkey),
		controlapi.TargetNode(RunOpts.TargetNode),
		controlapi.TargetPublicXKey(targetPublicXkey),
		controlapi.TriggerSubjects(RunOpts.TriggerSubjects),
		controlapi.WorkloadDescription(RunOpts.Description),
		controlapi.WorkloadName(RunOpts.Name),
		controlapi.WorkloadType(RunOpts.WorkloadType),
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
		fmt.Printf("✅ Workload '%s' stopped.\n", resp.ID)
	} else {
		fmt.Println("⛔ Workload failed to stop")
	}
}
