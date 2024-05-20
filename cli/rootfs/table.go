package rootfs

import (
	"fmt"
	"reflect"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

func (r RootfsOptions) Table() error {
	tw := table.NewWriter()
	tw.SetStyle(table.StyleRounded)

	tw.Style().Title.Align = text.AlignCenter
	tw.Style().Format.Header = text.FormatDefault

	tw.SetTitle("RootFS Configuration")
	tw.AppendHeader(table.Row{"Field", "Value", "Type"})
	tw.AppendRows([]table.Row{
		{"OutName", r.OutName, reflect.TypeOf(r.OutName)},
		{"BaseImage", r.BaseImage, reflect.TypeOf(r.BaseImage)},
		{"BuildScriptPath", r.BuildScriptPath, reflect.TypeOf(r.BuildScriptPath)},
		{"AgentBinaryPath", r.AgentBinaryPath, reflect.TypeOf(r.AgentBinaryPath)},
		{"RootFSSize", r.RootFSSize, reflect.TypeOf(r.RootFSSize)},
	})

	fmt.Println(tw.Render())
	return nil
}
