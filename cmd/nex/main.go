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
	defaultConfigPath string = "."

	VERSION   string = "0.0.0"
	COMMIT    string = "development"
	BUILDDATE string = time.Now().Format(time.RFC822)
)

func main() {
	userHomePath, err := os.UserConfigDir()
	if err != nil {
		userHomePath = "."
	}

	defaultConfigPath = filepath.Join(userHomePath, "nex")
	err = os.MkdirAll(defaultConfigPath, 0755)
	if err != nil {
		defaultConfigPath = "."
	}

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
			"defaultConfigPath":   defaultConfigPath,
			"defaultResourcePath": filepath.Join(defaultConfigPath, "bin"),
		},
		kong.BindTo(context.Background(), (*context.Context)(nil)),
		kong.Bind(&nex.Globals),
	)

	err = ctx.Run()
	ctx.FatalIfErrorf(err)
}
