package actors

import (
	"context"
	"errors"
	"log/slog"

	"disorder.dev/shandler"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func CreateNodeLogger(ctx context.Context, logger *slog.Logger) func() gen.ProcessBehavior {
	return func() gen.ProcessBehavior {
		return &NodeLogger{ctx: ctx, inner: logger}
	}
}

type NodeLogger struct {
	act.Actor

	ctx   context.Context
	inner *slog.Logger
}

func (nl *NodeLogger) HandleLog(message gen.MessageLog) error {
	attrs := getMsgLogAttrs(message.Source)
	args := append(message.Args, attrs...)

	switch message.Level {
	case gen.LogLevelTrace:
		nl.inner.Log(nl.ctx, shandler.LevelTrace, message.Format, args...)
		return nil
	case gen.LogLevelDebug:
		nl.inner.Debug(message.Format, args...)
		return nil
	case gen.LogLevelInfo:
		nl.inner.Info(message.Format, args...)
		return nil
	case gen.LogLevelWarning:
		nl.inner.Warn(message.Format, args...)
		return nil
	case gen.LogLevelError:
		nl.inner.Error(message.Format, args...)
		return nil
	case gen.LogLevelPanic:
		nl.inner.Log(nl.ctx, shandler.LevelFatal, message.Format, args...)
		return nil
	}

	return errors.New("failed to handle log message")
}

func getMsgLogAttrs(source any) []any {
	switch s := source.(type) {
	case gen.MessageLogNode:
		return []any{slog.String("node", s.Node.String()), slog.Int64("creation", s.Creation)}
	case gen.MessageLogProcess:
		return []any{slog.String("node", s.Node.String()), slog.String("pid", s.PID.String()), slog.String("name", s.Name.String()), slog.String("behavior", s.Behavior)}
	case gen.MessageLogMeta:
		return []any{slog.String("node", s.Node.String()), slog.String("parent", s.Parent.String()), slog.String("meta", s.Meta.String()), slog.String("behavior", s.Behavior)}
	case gen.MessageLogNetwork:
		return []any{slog.String("node", s.Node.String()), slog.String("peer", s.Peer.String()), slog.Int64("creation", s.Creation)}
	}
	return []any{}
}
