package ncontext

import "log/slog"

type ncontext struct {
	l *slog.Logger
}

func NewNContextView(l *slog.Logger) ncontext {
	return ncontext{
		l: l,
	}
}
