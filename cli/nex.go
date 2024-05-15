package main

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/alecthomas/kong"
	"github.com/alecthomas/kong-toml"
	"github.com/alecthomas/kong-yaml"
	shandler "github.com/jordan-rash/slog-handler"
	"github.com/nats-io/nats.go"
)

var (
	VERSION = "development"
)

type Context struct {
	config nexCLI

	logger *slog.Logger
}

func main() {
	logger := slog.New(shandler.NewHandler())

	cliCtx := kong.Parse(
		&nCLI,
		kong.Name("nex"),
		kong.Description("NATS Execution Engine CLI | Synadia Communications"),
		kong.ShortHelp(shortHelp),
		kong.ShortUsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true, NoExpandSubcommands: true, FlagsLast: true}),
		kong.Configuration(logConfig(kong.JSON, logger), "/etc/nex/config.json", "./nex.json"),
		kong.Configuration(logConfig(kongtoml.Loader, logger), "/etc/nex/config.toml", "./nex.toml"),
		kong.Configuration(logConfig(kongyaml.Loader, logger), "/etc/nex/config.yaml", "/etc/nex/config.yml", "./nex.yaml", "./nex.yml"),
		kong.Vars{
			"version":                VERSION,
			"default_nats_server":    nats.DefaultURL,
			"default_nats_conn_name": fmt.Sprintf("nex_%s", VERSION),
		},
	)

	logger = configLogger(nCLI.Global)

	err := cliCtx.Run(Context{nCLI, logger})
	cliCtx.FatalIfErrorf(err)
}

func logConfig(wrapped kong.ConfigurationLoader, logger *slog.Logger) kong.ConfigurationLoader {
	return func(r io.Reader) (kong.Resolver, error) {
		if n, ok := r.(interface{ Name() string }); ok {
			logger.Debug("loading config", slog.String("file", n.Name()))
		}
		return wrapped(r)
	}
}

func shortHelp(_ kong.HelpOptions, ctx *kong.Context) error {
	fmt.Fprintln(ctx.Stdout, "🤷 wish I could help")
	return nil
}
