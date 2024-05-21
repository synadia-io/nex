package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"time"

	shandler "github.com/jordan-rash/slog-handler"
	"github.com/nats-io/nats.go"

	"github.com/synadia-io/nex/cli/globals"
)

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
