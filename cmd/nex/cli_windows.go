package main

import (
	"github.com/alecthomas/kong"
)

type Globals struct {
	GlobalLogger `prefix:"logger." group:"Logger Configuration"`
	GlobalNats   `prefix:"nats." group:"NATS Configuration"`

	Config    kong.ConfigFlag  `help:"Configuration file to load" placeholder:"./nex.config.json"`
	Version   kong.VersionFlag `help:"Print version information"`
	Namespace string           `env:"NEX_NAMESPACE" placeholder:"default" help:"Specifies namespace when running nex commands"`
	Check     bool             `help:"Print the current values of all options without running a command"`
}

type NexCLI struct {
	Globals Globals `embed:""`

	Node     Node     `cmd:"" help:"Interact with execution engine nodes"`
	Workload Workload `cmd:"" help:"Interact with workloads"`
	Monitor  Monitor  `cmd:"" help:"Live monitor workload log emissions"`
	RootFS   RootFS   `cmd:"" name:"rootfs" help:"Build custom rootfs" alias:"fs"`
}

func (cli NexCLI) Validate() error {
	var errs error
	return errs
}
