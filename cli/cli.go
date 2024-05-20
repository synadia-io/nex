package main

type NexCLI struct {
	Global  globals         `embed:""`
	Node    nodeOptions     `cmd:"" help:"Interact with execution engine nodes" aliases:"nodes"`
	RunCmd  runOptions      `cmd:"" name:"run" help:"Run a workload on a target node"`
	Devrun  devRunOptions   `cmd:"" help:"Run a workload locating reasonable defaults (developer mode)" aliases:"yeet"`
	Stop    stopOptions     `cmd:"" help:"Stop a running workload"`
	Monitor monitorOptions  `cmd:"" help:"Monitor the status of events and logs" aliases:"watch"`
	Rootfs  rootfsOptions   `cmd:"" help:"Build custom rootfs" aliases:"fs"`
	Lame    lameDuckOptions `cmd:"" help:"Command a node to enter lame duck mode" aliases:"lameduck"`
	Upgrade upgradeOptions  `cmd:"" help:"Upgrade the NEX CLI to the latest version"`
}

func NewNexCLI() NexCLI {
	return NexCLI{}
}
