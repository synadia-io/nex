package main

import (
	"log/slog"
	"os"
	"strconv"

	nexui "github.com/synadia-io/nex/ui"
)

const (
	NEX_UI_PORT_ENV            = "NEX_UI_PORT"
	NEX_UI_HOST_ENV            = "NEX_UI_HOST"
	NEX_UI_NATS_SERVER_URL_ENV = "NEX_UI_NATS_SERVER_URL"
	NEX_UI_LOG_LEVEL_ENV       = "NEX_UI_LOG_LEVEL"
	NEX_UI_LOG_JSON_ENV        = "NEX_UI_LOG_JSON"
)

// This is a convenience binary if you just want to run the UI. The "normal"
// way of launching the UI (unless you're testing, etc) should be to use
// the UI subcommand from the nex CLI (`nex ui`)
func main() {
	opts := slog.HandlerOptions{}

	switch getEnv(NEX_UI_LOG_LEVEL_ENV, "error") {
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
	logJson, _ := strconv.ParseBool(getEnv(NEX_UI_LOG_JSON_ENV, "false"))
	if logJson {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &opts))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &opts))
	}

	uiport, err := strconv.ParseInt(getEnv(NEX_UI_PORT_ENV, "8080"), 10, 64)
	if err != nil {
		logger.Error("invalid port", err)
		return
	}

	uihost := getEnv(NEX_UI_HOST_ENV, "127.0.0.1")
	uinats := getEnv(NEX_UI_NATS_SERVER_URL_ENV, "nats://127.0.0.1:4222")

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
