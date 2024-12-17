//go:build !debug

package main

import "log/slog"

func pprof(_ *slog.Logger) {}
