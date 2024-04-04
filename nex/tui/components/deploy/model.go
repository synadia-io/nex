package deploy

import "log/slog"

type deploy struct {
	logger *slog.Logger
}

func NewDeployView(l *slog.Logger) deploy {
	return deploy{
		logger: l,
	}
}
