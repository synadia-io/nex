package monitor

import (
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

func (m MonitorOptions) Table() error {
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
