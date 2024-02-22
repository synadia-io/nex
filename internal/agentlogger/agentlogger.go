package agentlogger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

type LogLevel int32

const (
	LogLevelPanic LogLevel = 0
	LogLevelFatal LogLevel = 1
	LogLevelError LogLevel = 2
	LogLevelWarn  LogLevel = 3
	LogLevelInfo  LogLevel = 4
	LogLevelDebug LogLevel = 5
	LogLevelTrace LogLevel = 6
)

type LogEntry struct {
	Source string   `json:"source,omitempty"`
	Level  LogLevel `json:"level,omitempty"`
	Text   string   `json:"text,omitempty"`
}

const NexEventSourceNexAgent = "nex-agent"

func NewAgentLogger(ctx context.Context, nc *nats.Conn, md *agentapi.MachineMetadata) (*AgentLogger, error) {
	if nc == nil {
		return nil, errors.New("nats connection is nil")
	}

	if md == nil {
		return nil, errors.New("machine metadata is nil")
	}

	return &AgentLogger{
		entries: make(chan *LogEntry),
		nc:      nc,
		md:      md,
		ctx:     ctx,
	}, nil
}

// AgentLogger is a logger for the Next Agent that publishes logs to the Nats server of the node host
type AgentLogger struct {
	nc  *nats.Conn
	md  *agentapi.MachineMetadata
	ctx context.Context

	entries chan *LogEntry
}

// LogEntry logs a log entry
func (a *AgentLogger) LogEntry(entry *LogEntry) {
	select {
	case a.entries <- entry:
	case <-a.ctx.Done():
	}
}

// LogMessage is a convenience method for logging a message at the given level
func (a *AgentLogger) LogMessage(lvl LogLevel, msg string) {
	a.LogEntry(&LogEntry{
		Source: NexEventSourceNexAgent,
		Level:  lvl,
		Text:   msg,
	})
}

// Run pulls log entries off the channel and publish to the node host via internal NATS
func (a *AgentLogger) Run() {
	for {
		select {
		case entry := <-a.entries:
			if err := a.publishLog(entry); err != nil {
				fmt.Printf("error publishing agent log: %v\n", err)
			}
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *AgentLogger) publishLog(entry *LogEntry) error {
	bytes, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("agentint.%s.logs", *a.md.VmID)

	err = a.nc.Publish(subject, bytes)
	if err != nil {
		return err
	}

	err = a.nc.Flush()
	if err != nil {
		return err
	}

	return nil
}
