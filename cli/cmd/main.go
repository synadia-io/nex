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
	"github.com/nats-io/jsm.go/natscontext"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"

	"github.com/synadia-io/nex/cli"
	"github.com/synadia-io/nex/cli/globals"
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

	keypair, err := nkeys.CreateServer()
	if err != nil {
		logger.Error("failed to create nkey server keypair", slog.Any("err", err))
		return
	}
	pk, err := keypair.PublicKey()
	if err != nil {
		logger.Error("failed to extract public key", slog.Any("err", err))
		return
	}

	nCLI := cli.NewNexCLI(pk)
	cliCtx := kong.Parse(
		&nCLI,
		kong.Name("nex"),
		kong.Description("NATS Execution Engine CLI | Synadia Communications"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{Compact: true, NoExpandSubcommands: true, FlagsLast: true}),
		kong.Configuration(logConfig(kong.JSON, logger), "/etc/nex/config.json", "./nex.json"),
		kong.Configuration(logConfig(kongtoml.Loader, logger), "/etc/nex/config.toml", "./nex.toml"),
		kong.Configuration(logConfig(kongyaml.Loader, logger), "/etc/nex/config.yaml", "/etc/nex/config.yml", "./nex.yaml", "./nex.yml"),
		kong.Vars{
			"version":                fmt.Sprintf("v%s [%s] | BuiltOn: %s", VERSION, COMMIT, BUILDDATE),
			"default_nats_server":    nats.DefaultURL,
			"default_nats_conn_name": fmt.Sprintf("nex_%s", VERSION),
		},
	)

	nc, err := configureNatsConnection(nCLI.Global)
	if err != nil {
		cliCtx.FatalIfErrorf(err)
	}
	logger = configureLogger(nCLI.Global, nc, pk)

	cliCtx.BindTo(nc, (*nats.Conn)(nil))
	cliCtx.BindTo(ctx, (*context.Context)(nil))
	cliCtx.BindTo(logger, (*slog.Logger)(nil))
	err = cliCtx.Run(ctx, nc, logger, nCLI.Global)
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
