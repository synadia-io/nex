package main

type Monitor struct {
	Events Events `cmd:"" help:"Monitor events"`
	Logs   Logs   `cmd:"" help:"Monitor logs"`
}

// Monitor subcommands
type Events struct{}
type Logs struct{}
