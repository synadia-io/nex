package main

type Workload struct {
	Run  RunWorkload  `cmd:"" help:"Run a workload on a target node"`
	Stop StopWorkload `cmd:"" help:"Stop a running workload"`
}

// Workload subcommands
type RunWorkload struct{}
type StopWorkload struct{}
