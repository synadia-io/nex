package nodes

import "log/slog"

type nodes struct {
	logger *slog.Logger

	width, height int
}

func NewNodesView(l *slog.Logger) nodes {
	return nodes{
		logger: l,
	}
}
