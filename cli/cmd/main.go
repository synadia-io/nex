package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/alecthomas/kong"
	"github.com/alecthomas/kong-toml"
	"github.com/alecthomas/kong-yaml"
	shandler "github.com/jordan-rash/slog-handler"
	"github.com/nats-io/nats.go"

	"github.com/synadia-io/nex/cli"
)

const (
	VERSION   = "development"
	COMMIT    = "none"
	BUILDDATE = "unknown"
)

func main() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "VERSION", VERSION)      //nolint:staticcheck
	ctx = context.WithValue(ctx, "COMMIT", COMMIT)        //nolint:staticcheck
	ctx = context.WithValue(ctx, "BUILD_DATE", BUILDDATE) //nolint:staticcheck

	logger := slog.New(shandler.NewHandler())
	nCLI := cli.NewNexCLI(logger)

	cliCtx := kong.Parse(
		&nCLI,
		kong.Name("nex"),
		kong.Description("NATS Execution Engine CLI | Synadia Communications"),
		kong.ShortHelp(shortHelp),
		kong.UsageOnError(),
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

	// TODO: implement settings from config
	logger = slog.New(shandler.NewHandler())

	cliCtx.BindTo(ctx, (*context.Context)(nil))
	cliCtx.BindTo(logger, (*slog.Logger)(nil))
	err := cliCtx.Run(ctx, logger, nCLI.Global)
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
