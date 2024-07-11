//go:build !debug

package main

import "log/slog"

// no-op function
func initDebug(_ *slog.Logger) func() {
	return func() {}
}
