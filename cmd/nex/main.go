package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alecthomas/kong"
)

const (
	Banner = `
  	   ▐ ▄ ▄▄▄ . ▐▄• ▄ 
	  •█▌▐█▀▄.▀· █▌█▌▪
	  ▐█▐▐▌▐▀▀▪▄ ·██· 
	  ██▐█▌▐█▄▄▌▪▐█·█▌
	  ▀▀ █▪ ▀▀▀ •▀▀ ▀▀
`
)

var (
	VERSION   string = "0.0.0"
	COMMIT    string = "development"
	BUILDDATE string = time.Now().Format(time.RFC822)
)

func main() {
	userConfigPath, err := os.UserConfigDir()
	if err != nil {
		userConfigPath = "."
	}
	userResourcePath := filepath.Join(userConfigPath, ".nex", "bin")

	nex := new(NexCLI)
	ctx := kong.Parse(nex,
		kong.Name("nex"),
		kong.Description("The NATS execution engine\n"+Banner),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true, NoExpandSubcommands: true, FlagsLast: true}),
		kong.Configuration(kong.JSON),
		kong.Vars{
			"version":             fmt.Sprintf("%s [%s] | Built: %s", VERSION, COMMIT, BUILDDATE),
			"versionOnly":         VERSION,
			"defaultResourcePath": userResourcePath,
		},
		kong.NamedMapper("workloadConfigs", WorkloadConfigs{}),
	)

	ctx.BindTo(context.Background(), (*context.Context)(nil))
	ctx.BindTo(nex.Globals, (*Globals)(nil))

	err = ctx.Run()
	ctx.FatalIfErrorf(err)
}
