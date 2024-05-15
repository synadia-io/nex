package main

import (
	"fmt"
	"net/url"
	"reflect"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
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

type devRunOptions struct {
	Filename          string `required:"" default:"./path/to/file" type:"existingfile" help:"Path to workload file" group:"DevRun Configuration" json:"devrun_filename"`
	AutoStop          bool   `default:"true" help:"Stop a workload with the same name on a target" group:"DevRun Configuration" json:"devrun_autostop"`
	DevBucketMaxBytes int    `default:"104857600" help:"Max bytes override for when we create the NEXCLIFILES bucket" group:"DevRun Configuration" json:"devrun_bucket_max_bytes"` // 100MB default
	SharedRunOptions
}

func (d devRunOptions) Run(ctx Context) error {
	return nil
}

func (d devRunOptions) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("DevRun Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})

	tw.AppendRows([]table.Row{
		{"Filename", d.Filename, reflect.TypeOf(d.Filename)},
		{"AutoStop", d.AutoStop, reflect.TypeOf(d.AutoStop)},
		{"BucketMaxBytes", d.DevBucketMaxBytes, reflect.TypeOf(d.DevBucketMaxBytes)},
		{"Argv", d.Argv, reflect.TypeOf(d.Argv)},
		{"Env", d.Env, reflect.TypeOf(d.Env)},
		{"TargetNode", d.TargetNode, reflect.TypeOf(d.TargetNode)},
		{"Name", d.Name, reflect.TypeOf(d.Name)},
		{"Description", d.Description, reflect.TypeOf(d.Description)},
		{"WorkloadType", d.WorkloadType, reflect.TypeOf(d.WorkloadType)},
		{"PublisherXkeyFile", d.PublisherXkeyFile, reflect.TypeOf(d.PublisherXkeyFile)},
		{"ClaimsIssuerFile", d.ClaimsIssuerFile, reflect.TypeOf(d.ClaimsIssuerFile)},
		{"TriggerSubjects", d.TriggerSubjects, reflect.TypeOf(d.TriggerSubjects)},
	})

	fmt.Println(tw.Render())
	return nil
}

type runOptions struct {
	WorkloadUrl *url.URL `required:"" help:"URL to the workload" group:"Run Configuration" json:"run_workload_url"`
	Essential   bool     `default:"false" help:"Mark the workload as essential | Workloads will try and autorestart after failure" group:"Run Configuration" json:"run_essential"`
	SharedRunOptions
}

func (r runOptions) Run(ctx Context) error {
	return nil
}

func (r runOptions) Validate() error {
	return nil
}

func (r runOptions) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("Run Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})
	tw.AppendRows([]table.Row{
		{"WorkloadUrl", r.WorkloadUrl, reflect.TypeOf(r.WorkloadUrl)},
		{"Essential", r.Essential, reflect.TypeOf(r.Essential)},
		{"Argv", r.Argv, reflect.TypeOf(r.Argv)},
		{"Env", r.Env, reflect.TypeOf(r.Env)},
		{"TargetNode", r.TargetNode, reflect.TypeOf(r.TargetNode)},
		{"Name", r.Name, reflect.TypeOf(r.Name)},
		{"Description", r.Description, reflect.TypeOf(r.Description)},
		{"WorkloadType", r.WorkloadType, reflect.TypeOf(r.WorkloadType)},
		{"PublisherXkeyFile", r.PublisherXkeyFile, reflect.TypeOf(r.PublisherXkeyFile)},
		{"ClaimsIssuerFile", r.ClaimsIssuerFile, reflect.TypeOf(r.ClaimsIssuerFile)},
		{"TriggerSubjects", r.TriggerSubjects, reflect.TypeOf(r.TriggerSubjects)},
	})

	fmt.Println(tw.Render())
	return nil
}
