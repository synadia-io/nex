package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/synadia-labs/nex/models"

	"github.com/alecthomas/kong"
)

var (
	VERSION   = "0.0.0"
	COMMIT    = "development"
	BUILDDATE = time.Now().Format(time.DateTime)
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

	nex := new(NexCLI)
	ctx := kong.Parse(nex,
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
		kong.BindTo(context.Background(), (*context.Context)(nil)),
		kong.Bind(&nex.Globals),
	)

	err = ctx.Run()
	if err != nil {
		fmt.Println("error: ", err.Error())
	}
}
