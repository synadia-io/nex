package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/synadia-labs/nex/models"

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
	if err != nil && !errors.Is(err, models.ErrLameduckShutdown) {
		fmt.Println("error:", err.Error())
	}
}
