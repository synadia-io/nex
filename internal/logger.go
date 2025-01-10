package internal

import (
	"log/slog"
	"strings"
)

type agentLogCapture struct {
	logger *slog.Logger
	stderr bool
}

func (l agentLogCapture) Write(p []byte) (n int, err error) {
	if l.stderr {
		l.logger.Error(strings.ReplaceAll(string(p), "\n", ""))
	} else {
		l.logger.Info(strings.ReplaceAll(string(p), "\n", ""))
	}
	return len(p), nil
}
