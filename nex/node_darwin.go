//go:build darwin

package main

import (
	"context"
	"log/slog"

	"github.com/nats-io/nkeys"
)

func setConditionalCommands() {
	nodeUp = nodes.Command("up", "Starts a Nex node").Hidden()
	nodePreflight = nodes.Command("preflight", "Checks system for node requirements and installs missing").Hidden()
}

func RunNodeUp(ctx context.Context, logger *slog.Logger, keypair nkeys.KeyPair) error {
	return nil
}

func RunNodePreflight(ctx context.Context, logger *slog.Logger) error {
	return nil
}
