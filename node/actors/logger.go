package actors

import (
	"context"
	"fmt"
	"log/slog"

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
	nl.inner.Log(context.Background(), slog.Level(message.Level), fmt.Sprintf(message.Format, message.Args...))
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
