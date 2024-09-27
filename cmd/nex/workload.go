package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/nats-io/nkeys"
	"github.com/nats-io/nuid"
	"github.com/splode/fname"

	api "github.com/synadia-io/nex/api/agent/go/gen"
)

type Workload struct {
	Namespace string `default:"default" help:"Specifies namespace when running nex commands"`

	Run  RunWorkload  `cmd:"" help:"Run a workload on a target node" aliases:"start"`
	Stop StopWorkload `cmd:"" help:"Stop a running workload"`
}

func (w Workload) Validate() error {
	var errs error
	if w.Namespace == "" {
		errs = errors.Join(errs, errors.New("namespace must be specified"))
	}
	return errs
}

// Workload subcommands
type RunWorkload struct {
	File             string         `arg:"" required:"" help:"File containing workload to run"`
	TargetId         string         `help:"Node to run workload on"`
	Xkey             string         `default:"${defaultConfigPath}/publisher.xk" help:"Xkey file to use for workload"`
	IssuerKey        string         `default:"${defaultConfigPath}/issuer.nk" help:"Issuer key file to use for workload"`
	Name             string         `placeholder:"myworkload" help:"Name of the workload"`
	WorkloadType     string         `name:"type" default:"native" help:"Type of workload"`
	Description      string         `help:"Description of the workload"`
	Argv             []string       `placeholder:"--flag,value,-a,..." default:"" help:"Arguments to pass to the workload"`
	Env              map[string]any `placeholder:"KEY=VALUE;KEY2=VALUE2;..." default:"" help:"Environment variables to pass to the workload"`
	TriggerSubjects  []string       `placeholder:"subject1,subject2" default:"" help:"Trigger subjects to register for subsequent workload execution, if supported by the workload type"`
	HostServicesURL  string         `placeholder:"nats://localhost:4222" default:"" help:"NATS URL to host services"`
	HostServicesJWT  string         `placeholder:"etahsy..." default:"" help:"NATS Authentication JWT for host services"`
	HostServicesSeed string         `placeholder:"SUA..." default:"" help:"NATS Authentication Seed for host services"`
	Defaults         bool           `help:"Use default values for workload; chooses node at random"`
}

func (r *RunWorkload) AfterApply() error {
	if r.Argv == nil {
		r.Argv = []string{}
	}
	if r.Env == nil {
		r.Env = make(map[string]any)
	}
	if r.TriggerSubjects == nil {
		r.TriggerSubjects = []string{}
	}

	if _, err := os.Stat(r.Xkey); r.Defaults && os.IsNotExist(err) {
		kp, err := nkeys.CreatePair(nkeys.PrefixByteCurve)
		if err != nil {
			return err
		}
		seed, err := kp.Seed()
		if err != nil {
			return err
		}
		f, err := os.Create(r.Xkey)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.Write(seed)
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat(r.IssuerKey); r.Defaults && os.IsNotExist(err) {
		kp, err := nkeys.CreatePair(nkeys.PrefixByteAccount)
		if err != nil {
			return err
		}
		seed, err := kp.Seed()
		if err != nil {
			return err
		}
		f, err := os.Create(r.IssuerKey)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.Write(seed)
		if err != nil {
			return err
		}
	}

	if r.Defaults && r.Name == "" {
		rng := fname.NewGenerator()
		rName, err := rng.Generate()
		if err != nil {
			r.Name = "workload"
		}
		r.Name = rName
	}

	return nil
}

func (r RunWorkload) Validate() error {
	var errs error
	if _, err := os.Stat(r.File); os.IsNotExist(err) {
		errs = errors.Join(errs, errors.New("workload file does not exist"))
	}
	if _, err := os.Stat(r.Xkey); os.IsNotExist(err) {
		errs = errors.Join(errs, errors.New("xkey file does not exist"))
	}
	if _, err := os.Stat(r.IssuerKey); os.IsNotExist(err) {
		errs = errors.Join(errs, errors.New("issuer key file does not exist"))
	}
	if !r.Defaults {
		if r.Name == "" {
			errs = errors.Join(errs, errors.New("workload name must be provided"))
		}
		if r.TargetId == "" {
			errs = errors.Join(errs, errors.New("target-id must be specified"))
		}
		if r.WorkloadType == "" {
			errs = errors.Join(errs, errors.New("workload must be specified"))
		}
	}
	return errs
}
func (r RunWorkload) Run(ctx context.Context, globals Globals, workload *Workload) error {
	if globals.Check {
		return printTable("Node Run Workload Configuration", append(globals.Table(), append(workload.Table(), r.Table()...)...)...)
	}

	f, err := os.Open(r.File)
	if err != nil {
		return err
	}
	f_b, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	f.Close()
	sum := sha256.Sum256(f_b)

	startRequest := api.StartWorkloadRequestJson{
		Argv:            r.Argv,
		Env:             r.Env,
		Hash:            fmt.Sprintf("%x", sum),
		Name:            r.Name,
		Namespace:       workload.Namespace,
		TotalBytes:      len(f_b),
		TriggerSubjects: r.TriggerSubjects,
		WorkloadId:      nuid.New().Next(),
		WorkloadType:    r.WorkloadType,
	}

	// TODO: something with this struct
	fmt.Println(startRequest)
	return nil
}

type StopWorkload struct {
	WorkloadId string `arg:"" required:"" help:"ID of workload to stop"`
	TargetId   string `help:"Node to send stop workload on"`
	IssuerKey  string `default:"${defaultConfigPath}/issuer.nk" help:"Issuer key file to use for workload"`
	Reason     string `default:"" help:"Reason for stopping workload"`
	Immediate  bool   `default:"false" help:"Stop workload immediately; workload will try and gracefully stop otherwise"`

	Defaults bool `help:"Use default values for workload; chooses node at random"`
}

func (r StopWorkload) Validate() error {
	var errs error
	if r.WorkloadId == "" {
		errs = errors.Join(errs, errors.New("workload-id must be specified"))
	}
	if _, err := os.Stat(r.IssuerKey); os.IsNotExist(err) {
		errs = errors.Join(errs, errors.New("issuer key file does not exist"))
	}
	if !r.Defaults {
		if r.TargetId == "" {
			errs = errors.Join(errs, errors.New("target-id must be specified"))
		}
	}
	return errs
}
func (s StopWorkload) Run(ctx context.Context, globals Globals, workload *Workload) error {
	if globals.Check {
		return printTable("Node Run Workload Configuration", append(globals.Table(), append(workload.Table(), s.Table()...)...)...)
	}

	stopRequest := api.StopWorkloadRequestJson{
		Immediate:  &s.Immediate,
		Reason:     &s.Reason,
		WorkloadId: s.WorkloadId,
	}
	fmt.Println(stopRequest)
	return nil
}
