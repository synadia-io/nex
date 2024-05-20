package monitor

import (
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

func (e EventCmd) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("Monitor Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})
	tw.AppendRows([]table.Row{
		{"Node ID", e.sharedMonitorOptions.NodeId, "string"},
		{"Workload ID", e.sharedMonitorOptions.WorkloadId, "string"},
		{"Level", e.sharedMonitorOptions.Level, "string"},
	})
	fmt.Println(tw.Render())
	return nil
}

func (l LogCmd) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("Monitor Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})
	tw.AppendRows([]table.Row{
		{"Node ID", l.sharedMonitorOptions.NodeId, "string"},
		{"Workload ID", l.sharedMonitorOptions.WorkloadId, "string"},
		{"Level", l.sharedMonitorOptions.Level, "string"},
	})
	fmt.Println(tw.Render())
	return nil
}
