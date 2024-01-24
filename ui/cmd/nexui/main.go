package main

import (
	"log/slog"
	"os"
	"strconv"

	nexui "github.com/synadia-io/nex/ui"
)

const (
	nexUiPortEnv       = "NEX_UI_PORT"
	nexUiHostEnv       = "NEX_UI_HOST"
	nexUiNatsServerEnv = "NEX_UI_NATS_SERVER_URL"
	nexUiLogLevelEnv   = "NEX_UI_LOG_LEVEL"
	nexUiLogJsonEnv    = "NEX_UI_LOG_JSON"
)

// This is a convenience binary if you just want to run the UI. The "normal"
// way of launching the UI (unless you're testing, etc) should be to use
// the UI subcommand from the nex CLI (`nex ui`)
func main() {
	opts := slog.HandlerOptions{}

	switch getEnv(nexUiLogLevelEnv, "error") {
	case "debug":
		opts.Level = slog.LevelDebug
	case "info":
		opts.Level = slog.LevelInfo
	case "warn":
		opts.Level = slog.LevelWarn
	default:
		opts.Level = slog.LevelError
	}

	var logger *slog.Logger
	logJson, _ := strconv.ParseBool(getEnv(nexUiLogJsonEnv, "false"))
	if logJson {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &opts))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &opts))
	}

	uiport, err := strconv.ParseInt(getEnv(nexUiPortEnv, "8080"), 10, 64)
	if err != nil {
		logger.Error("invalid port", err)
		return
	}

	uihost := getEnv(nexUiHostEnv, "127.0.0.1")
	uinats := getEnv(nexUiNatsServerEnv, "nats://127.0.0.1:4222")

	ws := nexui.NewWebServer(
		nexui.WithLogger(logger),
		nexui.WithHost(uihost),
		nexui.WithPort(int(uiport)),
		nexui.WithNatsServer(uinats),
	)

	if err := ws.ServeUI(); err != nil {
		logger.Error("serve ui failed", err)
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
