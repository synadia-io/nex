package actors

import (
	"context"
	"log/slog"

	"disorder.dev/shandler"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

//TODO: figure out how log messages show up in the observer

func CreateNodeLogger(logger *slog.Logger) func() gen.ProcessBehavior {
	return func() gen.ProcessBehavior {
		return &NodeLogger{inner: logger}
	}
}

type NodeLogger struct {
	act.Actor

	inner *slog.Logger
}

func (nl *NodeLogger) HandleLog(message gen.MessageLog) error {
	// TODO: this is temporary until we can figure out what we really want
	// to do with these logs
	switch message.Level {
	case gen.LogLevelError:
		nl.inner.Error(message.Format, message.Args...)
	case gen.LogLevelWarning:
		nl.inner.Warn(message.Format, message.Args...)
	case gen.LogLevelInfo:
		nl.inner.Info(message.Format, message.Args...)
	case gen.LogLevelDebug:
		nl.inner.Debug(message.Format, message.Args...)
	case gen.LogLevelTrace:
		nl.inner.Log(context.TODO(), shandler.LevelTrace, message.Format, message.Args...)
	case gen.LogLevelPanic:
		nl.inner.Log(context.TODO(), shandler.LevelFatal, message.Format, message.Args...)
	}
	switch m := message.Source.(type) {
	case gen.MessageLogNode:
		// handle message
		nl.logFromNode(m, message)
	case gen.MessageLogProcess:
		nl.logFromProcess(m, message)
	case gen.MessageLogMeta:
		// handle message
		nl.logFromMeta(m, message)
	case gen.MessageLogNetwork:
		// handle message
		nl.logFromNetwork(m, message)
	}
	return nil
}

func (nl *NodeLogger) logFromNode(node gen.MessageLogNode, message gen.MessageLog) {

}

func (nl *NodeLogger) logFromProcess(proc gen.MessageLogProcess, message gen.MessageLog) {

}

func (nl *NodeLogger) logFromMeta(meta gen.MessageLogMeta, message gen.MessageLog) {

}

func (nl *NodeLogger) logFromNetwork(meta gen.MessageLogNetwork, message gen.MessageLog) {

}
