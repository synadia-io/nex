package run

import (
	"net/url"
)

type SharedRunOptions struct {
	Argv              []string          `help:"Arguments to pass to the workload" json:"sharedrun_argv"`
	Env               map[string]string `help:"Environment variables to pass to the workload" json:"sharedrun_env"`
	TargetNode        string            `help:"Node to run the workload on" json:"sharedrun_target_node"`
	Name              string            `help:"Name of the workload" json:"sharedrun_name"`
	Description       string            `help:"Description of the workload" json:"sharedrun_description"`
	WorkloadType      string            `help:"Type of workload" json:"sharedrun_workload_type"`
	PublisherXkeyFile []byte            `help:"Path to the publisher xkey file" type:"filecontent" json:"sharedrun_publisher_xkey_file"`
	ClaimsIssuerFile  []byte            `help:"Path to the claims issuer file" type:"filecontent" json:"sharedrun_claims_issuer_file"`
	TriggerSubjects   []string          `help:"Subjects to trigger function type workload" json:"sharedrun_trigger_subjects"`
}

type DevRunOptions struct {
	Filename          string `required:"" default:"./path/to/file" type:"existingfile" help:"Path to workload file" group:"DevRun Configuration" json:"devrun_filename"`
	AutoStop          bool   `default:"true" help:"Stop a workload with the same name on a target" group:"DevRun Configuration" json:"devrun_autostop"`
	DevBucketMaxBytes int    `default:"104857600" help:"Max bytes override for when we create the NEXCLIFILES bucket" group:"DevRun Configuration" json:"devrun_bucket_max_bytes"` // 100MB default
	SharedRunOptions
}

type RunOptions struct {
	WorkloadUrl *url.URL `required:"" help:"URL to the workload" group:"Run Configuration" json:"run_workload_url"`
	Essential   bool     `default:"false" help:"Mark the workload as essential | Workloads will try and autorestart after failure" group:"Run Configuration" json:"run_essential"`
	SharedRunOptions
}
