package preflight

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/fatih/color"
)

var (
	cyan    = color.New(color.FgCyan).SprintFunc()
	red     = color.New(color.FgRed).SprintFunc()
	magenta = color.New(color.FgMagenta).SprintFunc()
	green   = color.New(color.FgHiGreen).SprintFunc()

	nexLatestVersion = func(ctx context.Context, logger *slog.Logger) string {
		version := "development"
		build_data, ok := ctx.Value("build_data").(map[string]interface{})
		if ok {
			version, ok = build_data["version"].(string)
			if !ok {
				logger.Warn("error parsing version from build data")
			}
		}
		if version == "development" {
			backoff := 0

		RETRY:
			res, err := http.Get("https://api.github.com/repos/synadia-io/nex/releases/latest")
			if err != nil {
				logger.Error("error making http request", slog.Any("err", err))
				return ""
			}
			defer res.Body.Close()
			if res.StatusCode != 200 && backoff < 5 {
				slog.Warn("failed to get latest version, incrementing backoff", slog.String("status_code", res.Status), slog.Int("backoff", backoff))
				backoff++
				time.Sleep(time.Duration(backoff) * time.Second)
				goto RETRY
			} else if res.StatusCode != 200 {
				logger.Error("error getting latest version", slog.String("status_code", res.Status))
				return ""
			}

			b, err := io.ReadAll(res.Body)
			if err != nil {
				logger.Error("error reading body", slog.Any("err", err))
				return ""
			}

			payload := make(map[string]interface{})
			err = json.Unmarshal(b, &payload)
			if err != nil {
				slog.Error("error parsing json", slog.Any("err", err))
				return ""
			}

			latestTag, ok := payload["tag_name"].(string)
			if !ok {
				slog.Error("error parsing tag_name")
				return ""
			}
			return latestTag
		} else {
			return version
		}
	}
)
