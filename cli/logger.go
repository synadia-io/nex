package main

import "log/slog"
import shandler "github.com/jordan-rash/slog-handler"

// TODO: configure logger with options
func configLogger(_ globals) *slog.Logger {
	return slog.New(shandler.NewHandler())
}
