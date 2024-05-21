package cli

import (
	"github.com/synadia-io/nex/cli/globals"
	"github.com/synadia-io/nex/cli/lameduck"
	"github.com/synadia-io/nex/cli/monitor"
	"github.com/synadia-io/nex/cli/node"
	"github.com/synadia-io/nex/cli/rootfs"
	"github.com/synadia-io/nex/cli/run"
	"github.com/synadia-io/nex/cli/stop"
	"github.com/synadia-io/nex/cli/upgrade"
)

type NexCLI struct {
	Global  globals.Globals          `embed:""`
	Node    node.NodeOptions         `cmd:"" help:"Interact with execution engine nodes" aliases:"nodes"`
	RunCmd  run.RunOptions           `cmd:"" name:"run" help:"Run a workload on a target node"`
	Devrun  run.DevRunOptions        `cmd:"" help:"Run a workload locating reasonable defaults (developer mode)" aliases:"yeet"`
	Stop    stop.StopOptions         `cmd:"" help:"Stop a running workload"`
	Monitor monitor.MonitorOptions   `cmd:"" help:"Monitor the status of events and logs" aliases:"watch"`
	Rootfs  rootfs.RootfsOptions     `cmd:"" help:"Build custom rootfs" aliases:"fs"`
	Lame    lameduck.LameDuckOptions `cmd:"" help:"Command a node to enter lame duck mode" aliases:"lameduck"`
	Upgrade upgrade.UpgradeOptions   `cmd:"" help:"Upgrade the NEX CLI to the latest version"`
}

func NewNexCLI(pk string) NexCLI {
	cli := NexCLI{}
	cli.Node.ServerPublicKey = pk
	return cli
}
