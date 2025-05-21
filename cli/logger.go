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
	"github.com/synadia-labs/nex/models"
)

func configureLogger(globals *Globals, nc *nats.Conn, serverPublicKey string, showWorkloadLogs bool) *slog.Logger {
	var handlerOpts []shandler.HandlerOption

	filter := []string{}
	if !showWorkloadLogs {
		filter = append(filter, "workload")
	}

	handlerOpts = append(handlerOpts, shandler.WithGroupFilter(filter))

	if globals.LogShortLevels {
		handlerOpts = append(handlerOpts, shandler.WithShortLevels())
	}

	if globals.LogGroupOnRight {
		handlerOpts = append(handlerOpts, shandler.WithGroupRightJustify())
	}

	switch globals.LogLevel {
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

	switch globals.LogTimeFormat {
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

	if globals.JSON {
		handlerOpts = append(handlerOpts, shandler.WithJSON())
	}

	if globals.LogColor {
		handlerOpts = append(handlerOpts, shandler.WithColor())
	}

	if globals.LogWithPid {
		handlerOpts = append(handlerOpts, shandler.WithPid())
	}

	stdoutWriters := []io.Writer{}
	stderrWriters := []io.Writer{}

	if slices.Contains(globals.Target, "std") {
		stdoutWriters = append(stdoutWriters, os.Stdout)
		stderrWriters = append(stderrWriters, os.Stderr)
	}
	if slices.Contains(globals.Target, "file") {
		stdout, err := os.Create("nex.log")
		if err == nil {
			stderr, err := os.Create("nex.err")
			if err == nil {
				stdoutWriters = append(stdoutWriters, stdout)
				stderrWriters = append(stderrWriters, stderr)
			}
		}
	}
	if slices.Contains(globals.Target, "nats") && nc != nil {
		natsLogSubject := fmt.Sprintf("%s.%s.stdout", models.LogAPIPrefix(models.NodeSystemNamespace), serverPublicKey)
		natsErrLogSubject := fmt.Sprintf("%s.%s.stderr", models.LogAPIPrefix(models.NodeSystemNamespace), serverPublicKey)
		stdoutWriters = append(stdoutWriters, NewNatsLogger(nc, natsLogSubject))
		stderrWriters = append(stderrWriters, NewNatsLogger(nc, natsErrLogSubject))
	}

	handlerOpts = append(handlerOpts, shandler.WithStdOut(stdoutWriters...))
	handlerOpts = append(handlerOpts, shandler.WithStdErr(stderrWriters...))

	handlerOpts = append(handlerOpts, shandler.WithTextOutputFormat("%[2]s [%[1]s] %[3]s\n"))

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
