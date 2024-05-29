package main

import (
	"context"
	"fmt"
	"github.com/alecthomas/kong"
	"github.com/alecthomas/kong-toml"
	"github.com/alecthomas/kong-yaml"
	shandler "github.com/jordan-rash/slog-handler"
	"github.com/nats-io/jsm.go/natscontext"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/internal/cli"
	"github.com/synadia-io/nex/internal/cli/globals"
	"io"
	"log/slog"
	"os"
	"slices"
	"time"
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

	nCLI := cli.NewNexCLI(keypair)
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
	err = cliCtx.Run(ctx, nc, logger, nCLI.Global, nCLI.Node)
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

func configureLogger(cfg globals.Globals, nc *nats.Conn, serverPublicKey string) *slog.Logger {
	var handlerOpts []shandler.HandlerOption

	switch cfg.LogLevel {
	case "debug":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelDebug))
	case "info":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelInfo))
	case "warn":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelWarn))
	default:
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelError))
	}

	switch cfg.LogTimeFormat {
	case "DateOnly":
		handlerOpts = append(handlerOpts, shandler.WithTimeFormat(time.DateOnly))
	case "Stamp":
		handlerOpts = append(handlerOpts, shandler.WithTimeFormat(time.Stamp))
	case "RFC822":
		handlerOpts = append(handlerOpts, shandler.WithTimeFormat(time.RFC822))
	case "RFC3339":
		handlerOpts = append(handlerOpts, shandler.WithTimeFormat(time.RFC3339))
	default:
		handlerOpts = append(handlerOpts, shandler.WithTimeFormat(time.DateTime))
	}

	if cfg.LogJSON {
		handlerOpts = append(handlerOpts, shandler.WithJSON())
	}

	if cfg.LogsColorized {
		handlerOpts = append(handlerOpts, shandler.WithColor())
	}

	stdoutWriters := []io.Writer{}
	stderrWriters := []io.Writer{}

	if slices.Contains(cfg.Logger, "std") {
		stdoutWriters = append(stdoutWriters, os.Stdout)
		stderrWriters = append(stderrWriters, os.Stderr)
	}
	if slices.Contains(cfg.Logger, "file") {
		stdout, err := os.Create("nex.log")
		if err == nil {
			stderr, err := os.Create("nex.err")
			if err == nil {
				stdoutWriters = append(stdoutWriters, stdout)
				stderrWriters = append(stderrWriters, stderr)
			}
		}
	}
	if slices.Contains(cfg.Logger, "nats") {
		natsLogSubject := fmt.Sprintf("$NEX.logs.%s.stdout", serverPublicKey)
		natsErrLogSubject := fmt.Sprintf("$NEX.logs.%s.stderr", serverPublicKey)
		stdoutWriters = append(stdoutWriters, NewNatsLogger(nc, natsLogSubject))
		stderrWriters = append(stderrWriters, NewNatsLogger(nc, natsErrLogSubject))
	}

	handlerOpts = append(handlerOpts, shandler.WithStdOut(stdoutWriters...))
	handlerOpts = append(handlerOpts, shandler.WithStdErr(stderrWriters...))

	return slog.New(shandler.NewHandler(handlerOpts...))
}

type NatsLogger struct {
	nc       *nats.Conn
	OutTopic string
}

func NewNatsLogger(nc *nats.Conn, topic string) *NatsLogger {
	return &NatsLogger{
		OutTopic: topic,
		nc:       nc,
	}
}

func (nl *NatsLogger) Write(p []byte) (int, error) {
	err := nl.nc.Publish(nl.OutTopic, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
func configureNatsConnection(cfg globals.Globals) (*nats.Conn, error) {
	if cfg.Check {
		return nil, nil
	}

	opts := []nats.Option{
		nats.Name(cfg.ConnectionName),
	}

	// If a nats context is provided, it takes priority
	if !natscontext.IsKnown(cfg.NatsContext) {
		nc, err := natscontext.Connect(cfg.NatsContext, opts...)
		if err != nil {
			return nil, err
		}
		return nc, nil
	}

	if cfg.Creds != "" {
		opts = append(opts, nats.UserCredentials(cfg.Creds))
	}

	if cfg.Nkey == "" {
		opts = append(opts, nats.Nkey(cfg.Nkey, nats.GetDefaultOptions().SignatureCB))
	}

	if cfg.TlsCert != "" && cfg.TlsKey != "" {
		opts = append(opts, nats.ClientCert(cfg.TlsCert, cfg.TlsKey))
	}

	if cfg.TlsCA != "" {
		opts = append(opts, nats.RootCAs(cfg.TlsCA))
	}

	if cfg.TlsFirst {
		opts = append(opts, nats.TLSHandshakeFirst())
	}

	if cfg.Username != "" && cfg.Password == "" {
		opts = append(opts, nats.Token(cfg.Username))
	} else {
		opts = append(opts, nats.UserInfo(cfg.Username, cfg.Password))
	}

	nc, err := nats.Connect(cfg.Server, opts...)
	if err != nil {
		return nil, err
	}
	return nc, nil
}
