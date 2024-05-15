package main

import (
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

type monitorOptions struct {
	Log   LogCmd   `cmd:"" help:"Live monitor workload log emissions" aliases:"logs"`
	Event EventCmd `cmd:"" help:"Live monitor events from nex nodes" aliases:"events"`

	NodeId     string `placeholder:"SNEXISCOOL..." help:"Node to monitor" group:"Monitor Configuration"`
	WorkloadId string `placeholder:"<workload_id>" help:"Workload to monitor" group:"Monitor Configuration"`
	Level      string `default:"info" enum:"debug,warn,info,error" help:"Level of logs to monitor" group:"Monitor Configuration"`
}

func (m monitorOptions) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("Monitor Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})
	tw.AppendRows([]table.Row{
		{"Node ID", m.NodeId, "string"},
		{"Workload ID", m.WorkloadId, "string"},
		{"Level", m.Level, "string"},
	})
	fmt.Println(tw.Render())
	return nil
}

type LogCmd struct{}

func (l LogCmd) Run(ctx Context) error {
	return nil
}

type EventCmd struct{}

func (e EventCmd) Run(ctx Context) error {
	return nil
}
