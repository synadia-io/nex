package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/synadia-io/nex/models"

	"github.com/alecthomas/kong"
)

var (
	VERSION   = "0.0.0"
	COMMIT    = "development"
	BUILDDATE = "unknown"
)

type NexCLI struct {
	Globals Globals `embed:""`

	Node     Node     `cmd:"" help:"Interact with execution engine nodes"`
	Workload Workload `cmd:"" help:"Interact with workloads" aliases:"workloads"`
}

func main() {
	userConfigPath, err := os.UserConfigDir()
	if err != nil {
		userConfigPath = "."
	}
	userResourcePath := filepath.Join(userConfigPath, "nex")

	ctx := context.Background()
	ctx = context.WithValue(ctx, "VERSION", VERSION)     //nolint
	ctx = context.WithValue(ctx, "COMMIT", COMMIT)       //nolint
	ctx = context.WithValue(ctx, "BUILDDATE", BUILDDATE) //nolint

	nex := new(NexCLI)
	kctx := kong.Parse(nex,
		kong.Name("nex"),
		kong.Description("The NATS Execution Engine\n"+banner),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true, NoExpandSubcommands: true, FlagsLast: true}),
		kong.Configuration(kong.JSON, "/etc/nex/config.json", filepath.Join(userResourcePath, "config.json"), "./config.json"),
		kong.Vars{
			"version":             fmt.Sprintf("%s [%s] | Built: %s", VERSION, COMMIT, BUILDDATE),
			"versionOnly":         VERSION,
			"defaultResourcePath": userResourcePath,
			"adminNamespace":      models.NodeSystemNamespace,
		},
		kong.BindTo(ctx, (*context.Context)(nil)),
		kong.Bind(&nex.Globals),
	)

	err = kctx.Run()
	switch {
	case err == nil, errors.Is(err, models.ErrLameduckShutdown):
		// Clean exit. Lameduck shutdown is a requested stop, not a failure.
	case errors.Is(err, errSilentExit):
		// The command already reported the outcome on stdout/stderr (e.g. a
		// --json payload, or an "update not yet applied" line); this only
		// carries the non-zero exit so scripts and CI can detect it.
		os.Exit(1)
	default:
		// A printed error with exit 0 is invisible to scripts and CI.
		fmt.Println("error:", err.Error())
		os.Exit(1)
	}
}

// errSilentExit makes a command exit non-zero without main printing an
// "error:" line, for outcomes the command has already reported in full (a
// --json payload the caller will parse, or a human-readable status line).
var errSilentExit = errors.New("command failed")
