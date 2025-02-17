package internal

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
)

func TestLogCapture(t *testing.T) {
	today := time.Now().Format(time.DateOnly)

	stdout := new(bytes.Buffer)
	l := agentLogCapture{
		logger: slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == "time" {
					a.Value = slog.AnyValue(time.Now().Format(time.DateOnly))
				}
				return a
			},
		})),
		stderr: false,
	}

	size, err := l.Write([]byte("test"))
	be.NilErr(t, err)
	be.Equal(t, 4, size)
	be.Equal(t, fmt.Sprintf("time=%s level=INFO msg=test\n", today), stdout.String())
}
