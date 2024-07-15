//go:build !debug

package main

import "log/slog"

func setDebuggerCommands() {
	// no-op
}

// no-op function
func initDebug(_ *slog.Logger) (func() error, error) {
	return func() error { return nil }, nil
}
