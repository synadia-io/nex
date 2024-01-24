//go:build !(linux && amd64)

package main

import (
	"context"
	"log/slog"
)

func setConditionalCommands() {
	node_up = nodes.Command("up", "Starts a NEX node").Hidden()
	node_preflight = nodes.Command("preflight", "Checks system for node requirements and installs missing").Hidden()
}

func RunNodeUp(ctx context.Context, logger *slog.Logger) error {
	return nil
}

func RunNodePreflight(ctx context.Context, logger *slog.Logger) error {
	return nil
}
