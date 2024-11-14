//go:build !windows

package main

import (
	"errors"

	"github.com/alecthomas/kong"
)

type Globals struct {
	GlobalLogger `prefix:"logger." group:"Logger Configuration"`
	GlobalNats   `prefix:"nats." group:"NATS Configuration"`

	Config              kong.ConfigFlag  `help:"Configuration file to load" placeholder:"./nex.config.json"`
	Version             kong.VersionFlag `help:"Print version information"`
	Namespace           string           `env:"NEX_NAMESPACE" default:"default" help:"Specifies namespace when running nex commands"`
	Check               bool             `help:"Print the current values of all options without running a command"`
	DisableUpgradeCheck bool             `env:"NEX_DISABLE_UPGRADE_CHECK" name:"disable-upgrade-check" help:"Disable the upgrade check"`
	AutoUpgrade         bool             `env:"NEX_AUTO_UPGRADE" name:"auto-upgrade" help:"Automatically upgrade the nex CLI when a new version is available"`
}

type NexCLI struct {
	Globals Globals `embed:"" group:"Global Options"`

	Node     Node     `cmd:"" help:"Interact with execution engine nodes"`
	Workload Workload `cmd:"" help:"Interact with workloads" aliases:"workloads"`
	Monitor  Monitor  `cmd:"" help:"Live monitor workload log emissions"`
	RootFS   RootFS   `cmd:"" name:"rootfs" help:"Build custom rootfs" aliases:"fs"`
	Upgrade  Upgrade  `cmd:"" help:"Upgrade the NEX CLI to the latest version"`
}

func (cli NexCLI) Validate() error {
	var errs error
	if cli.Globals.DisableUpgradeCheck && cli.Globals.AutoUpgrade {
		errs = errors.Join(errs, errors.New("cannot enable auto-upgrade when upgrade check is disabled"))
	}
	return errs
}
