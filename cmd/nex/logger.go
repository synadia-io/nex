package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"time"

	"disorder.dev/shandler"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/models"
)

func configureLogger(cfg *Globals, nc *nats.Conn, serverPublicKey string, showSystemLogs, hideWorkloadLogs bool) *slog.Logger {
	var handlerOpts []shandler.HandlerOption

	filter := []string{}
	if !showSystemLogs {
		filter = append(filter, "actor_system")
	}
	if hideWorkloadLogs {
		filter = append(filter, "workload")
	}

	handlerOpts = append(handlerOpts, shandler.WithGroupFilter(filter))

	if cfg.LogShortLevels {
		handlerOpts = append(handlerOpts, shandler.WithShortLevels())
	}

	switch cfg.LogLevel {
	case "debug":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelDebug))
	case "info":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelInfo))
	case "warn":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelWarn))
	case "trace":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(shandler.LevelTrace))
	case "fatal":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(shandler.LevelFatal))
	default:
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelError))
	}

	switch cfg.LogTimeFormat {
	case "TimeOnly":
		handlerOpts = append(handlerOpts, shandler.WithTimeFormat(time.TimeOnly))
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

	if cfg.LogColor {
		handlerOpts = append(handlerOpts, shandler.WithColor())
	}

	stdoutWriters := []io.Writer{}
	stderrWriters := []io.Writer{}

	if slices.Contains(cfg.Target, "std") {
		stdoutWriters = append(stdoutWriters, os.Stdout)
		stderrWriters = append(stderrWriters, os.Stderr)
	}
	if slices.Contains(cfg.Target, "file") {
		stdout, err := os.Create("nex.log")
		if err == nil {
			stderr, err := os.Create("nex.err")
			if err == nil {
				stdoutWriters = append(stdoutWriters, stdout)
				stderrWriters = append(stderrWriters, stderr)
			}
		}
	}
	if slices.Contains(cfg.Target, "nats") {
		natsLogSubject := fmt.Sprintf("%s.%s.%s.stdout", models.LogAPIPrefix, models.NodeSystemNamespace, serverPublicKey)
		natsErrLogSubject := fmt.Sprintf("%s.%s.%s.stderr", models.LogAPIPrefix, models.NodeSystemNamespace, serverPublicKey)
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
