package main

import (
	"fmt"
	"reflect"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

type rootfsOptions struct {
	OutName         string `default:"rootfs.ext4.gz" required:"" help:"Name of the output file" group:"RootFS Configuration" json:"rootfs_outfile"`
	BaseImage       string `default:"synadia/nex-rootfs:alpine" required:"" help:"Base image to use for the rootfs" group:"RootFS Configuration" json:"rootfs_base_image"`
	BuildScriptPath string `placeholder:"script.sh" help:"Base image to use for the rootfs" group:"RootFS Configuration" json:"rootfs_build_script"`
	AgentBinaryPath string `placeholder:"../path/to/nex-agent" required:"" help:"Path to the agent binary" group:"RootFS Configuration" json:"rootfs_agent_binary"`
	RootFSSize      int    `default:"157286400" help:"Size of the rootfs in bytes" group:"RootFS Configuration" json:"rootfs_size"` //150MB default
}

func (r rootfsOptions) Table() error {
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

func (r rootfsOptions) Run(ctx Context) error {
	return nil
}
