package main

import (
	"fmt"
	"reflect"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

type stopOptions struct {
	WorkloadId           string `required:"" placeholder:"<workload_id>" help:"ID of the workload to stop" group:"Stop Configuration" json:"stop_workload_id"`
	TargetNode           string `required:"" placeholder:"SDERPMON..." help:"Node to stop the workload on" group:"Stop Configuration" json:"stop_target_node"`
	ClaimsIssuerFilePath string `required:"" placeholder:"./path/to/nkey.nk" help:"Path to the claims issuer nkey file." group:"Stop Configuration" json:"stop_claims_issuer_file"`
}

func (s stopOptions) Run(ctx Context) error {
	return nil
}

func (s stopOptions) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("DevRun Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})

	tw.AppendRows([]table.Row{
		{"WorkloadId", s.WorkloadId, reflect.TypeOf(s.WorkloadId)},
		{"TargetNode", s.TargetNode, reflect.TypeOf(s.TargetNode)},
		{"ClaimsIssuerFilePath", s.ClaimsIssuerFilePath, reflect.TypeOf(s.ClaimsIssuerFilePath)},
	})

	fmt.Println(tw.Render())
	return nil
}
