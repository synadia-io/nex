package cli

import (
	"errors"
	"log/slog"

	shandler "github.com/jordan-rash/slog-handler"

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
	logger *slog.Logger

	Global  globals.Globals          `embed:""`
	Node    node.NodeOptions         `cmd:"" help:"Interact with execution engine nodes" aliases:"nodes"`
	RunCmd  run.RunOptions           `cmd:"" name:"run" help:"Run a workload on a target node"`
	Devrun  run.DevRunOptions        `cmd:"" help:"Run a workload locating reasonable defaults (developer mode)" aliases:"yeet"`
	Stop    stop.StopOptions         `cmd:"" help:"Stop a running workload"`
	Monitor monitor.MonitorOptions   `cmd:"" help:"Monitor the status of events and logs" aliases:"watch"`
	Rootfs  rootfs.RootfsOptions     `cmd:"" help:"Build custom rootfs" aliases:"fs"`
	Lame    lameduck.LameDuckOptions `cmd:"" help:"Command a node to enter lame duck mode" aliases:"lameduck"`
	Upgrade upgrade.UpgradeOptions   `cmd:"" help:"Upgrade the NEX CLI to the latest version"`
	Check   CheckConfigCmd           `default:"1" cmd:"" hidden:""`
}

func NewNexCLI(logger *slog.Logger) NexCLI {
	return NexCLI{
		logger: logger,
	}
}

func (n *NexCLI) UpdateLogger() error {
	// TODO: implement settings from config
	n.logger = slog.New(shandler.NewHandler())
	return nil
}

type CheckConfigCmd struct{}

func (n CheckConfigCmd) Run(cfg globals.Globals) error {
	if cfg.Check {
		return cfg.Table()
	}
	return errors.New("invalid command")
}
