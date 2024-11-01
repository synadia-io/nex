package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/api/nodecontrol"
	"github.com/synadia-io/nex/api/nodecontrol/gen"
)

var validURIPrefix []string = []string{"nats://", "file://", "oci://"}

type Workload struct {
	Run  RunWorkload  `cmd:"" help:"Run a workload on a target node" aliases:"start,deploy"`
	Stop StopWorkload `cmd:"" help:"Stop a running workload" aliases:"undeploy"`
}

type NatsCreds struct {
	NatsUrl      string `placeholder:"nats://localhost:4222"`
	NatsUserJwt  string `placeholder:"ENATSJWTABC..."`
	NatsUserSeed string `placeholder:"SUAEY2LZ..."`
}

// Workload subcommands
type RunWorkload struct {
	NodeId string `description:"Node ID to run the workload on"`

	WorkloadName             string            `name:"name" description:"Name of the workload"`
	WorkloadArguments        []string          `name:"argv" description:"Arguments to pass to the workload"`
	WorkloadEnvironment      map[string]string `name:"env" description:"Environment variables to set for the workload"`
	WorkloadDescription      string            `name:"description" description:"Description of the workload"`
	WorkloadEssential        bool              `name:"essential" description:"Is the workload essential" default:"false"`
	WorkloadHash             string            `name:"hash" description:"Hash of the workload. If provided, will use for validation, if not provided, will calculate"`
	WorkloadHostServiceCreds NatsCreds         `embed:"" prefix:"hostservices." description:"Host service configuration"`
	WorkloadJsDomain         string            `name:"jsdomain" description:"JS Domain to run the workload under"`
	WorkloadRetryCount       int               `name:"retry-count" description:"Number of times to retry the workload" default:"3"`
	WorkloadPublicKey        string            `name:"public-key" description:"Public key of the workload"`
	WorkloadUri              string            `name:"uri" description:"URI of the workload.  file:// oci:// nats://" placeholder:"file://./workload"`
	WorkloadTriggerSubjects  []string          `name:"triggers" description:"Subjects to trigger the workload"`
	WorkloadType             string            `name:"type" description:"Type of workload" default:"direct_start"`
}

func (RunWorkload) AfterApply(globals *Globals) error {
	return checkVer(globals)
}

func (r RunWorkload) Validate() error {
	var errs error

	if !nkeys.IsValidPublicServerKey(r.NodeId) {
		errs = errors.Join(errs, errors.New("Node ID is not a valid public server key"))
	}

	if r.WorkloadUri == "" {
		errs = errors.Join(errs, errors.New("Workload URI is required"))
	}

	validPrefix := false
	for _, prefix := range validURIPrefix {
		if strings.HasPrefix(r.WorkloadUri, prefix) {
			validPrefix = true
			break
		}
	}
	if !validPrefix {
		errs = errors.Join(errs, errors.New("Workload URI must start with one of the following prefixes: "+strings.Join(validURIPrefix, ", ")))
	}

	return errs
}

func (r RunWorkload) Run(ctx context.Context, globals *Globals, w *Workload) error {
	if globals.Check {
		return printTable("Run Workload Configuration", append(globals.Table(), r.Table()...)...)
	}

	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	controller, err := nodecontrol.NewControlApiClient(nc, slog.New(slog.NewTextHandler(os.Stdin, nil)))
	if err != nil {
		return err
	}

	var env string
	if r.WorkloadEnvironment != nil {
		env_b, err := json.Marshal(r.WorkloadEnvironment)
		if err != nil {
			return err
		}
		env = base64.StdEncoding.EncodeToString(env_b)
	}

	startRequest := gen.StartWorkloadRequestJson{
		Argv:        r.WorkloadArguments,
		Description: r.WorkloadDescription,
		Environment: env,
		Essential:   r.WorkloadEssential,
		Hash:        r.WorkloadHash,
		HostServiceConfig: gen.HostServicesConfig{
			NatsUrl:      r.WorkloadHostServiceCreds.NatsUrl,
			NatsUserJwt:  r.WorkloadHostServiceCreds.NatsUserJwt,
			NatsUserSeed: r.WorkloadHostServiceCreds.NatsUserSeed,
		},
		Jsdomain:        r.WorkloadJsDomain,
		RetryCount:      r.WorkloadRetryCount,
		SenderPublicKey: r.WorkloadPublicKey,
		TriggerSubjects: r.WorkloadTriggerSubjects,
		Uri:             r.WorkloadUri,
		WorkloadName:    r.WorkloadName,
		WorkloadType:    r.WorkloadType,
	}

	resp, err := controller.DeployWorkload(globals.Namespace, r.NodeId, startRequest)
	if err != nil {
		return err
	}

	if resp.Started {
		fmt.Printf("Workload %s [%s] started on node %s\n", r.WorkloadName, resp.Id, r.NodeId)
	} else {
		fmt.Printf("Workload %s failed to start on node %s\n", r.WorkloadName, r.NodeId)
	}

	return nil
}

type StopWorkload struct {
	NodeId       string `description:"Node ID of the node the workload in running"`
	WorkloadId   string `description:"ID of the workload to stop" required:"true"`
	WorkloadType string `name:"type" description:"Type of workload" default:"direct_start"`
}

func (StopWorkload) AfterApply(globals *Globals) error {
	return checkVer(globals)
}

func (s StopWorkload) Validate() error {
	var errs error
	return errs
}

func (s StopWorkload) Run(ctx context.Context, globals *Globals, w *Workload) error {
	if globals.Check {
		return printTable("Stop Workload Configuration", append(globals.Table(), s.Table()...)...)
	}

	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	controller, err := nodecontrol.NewControlApiClient(nc, slog.New(slog.NewTextHandler(os.Stdin, nil)))
	if err != nil {
		return err
	}

	req := gen.StopWorkloadRequestJson{
		NodeId:       s.NodeId,
		WorkloadId:   s.WorkloadId,
		WorkloadType: s.WorkloadType,
	}

	resp, err := controller.UndeployWorkload(s.NodeId, globals.Namespace, req)
	if err != nil {
		return err
	}

	if resp.Stopped {
		fmt.Printf("Workload %s stopped on node %s\n", s.WorkloadId, s.NodeId)
	} else {
		fmt.Printf("Workload %s failed to stop on node %s\n", s.WorkloadId, s.NodeId)
	}

	return nil
}
