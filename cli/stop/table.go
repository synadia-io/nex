package stop

import (
	"fmt"
	"reflect"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

func (s StopOptions) Table() error {
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
