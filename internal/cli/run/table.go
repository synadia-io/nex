package run

import (
	"fmt"
	"reflect"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

func (d DevRunOptions) Table() error {
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

func (r RunOptions) Table() error {
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
