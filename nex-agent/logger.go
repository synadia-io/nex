package nexagent

import (
	"encoding/json"
	"fmt"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

// logEmitter implements the writer interface that allows us to capture a workload's
// stdout and stderr so that we can then publish those logs to the host node
type logEmitter struct {
	name   string
	stderr bool

	logs chan *agentapi.LogEntry
}

// Write arbitrary bytes to the underlying log emitter
func (l *logEmitter) Write(bytes []byte) (int, error) {
	var lvl agentapi.LogLevel
	if l.stderr {
		lvl = agentapi.LogLevelError
	} else {
		lvl = agentapi.LogLevelInfo
	}

	l.logs <- &agentapi.LogEntry{
		Level:  lvl,
		Source: l.name,
		Text:   string(bytes),
	}

	// FIXME-- this never returns an error
	return len(bytes), nil
}

// Run inside a goroutine to pull event entries and publish them to the node host.
func (a *Agent) dispatchEvents() {
	for {
		entry := <-a.eventLogs
		bytes, err := json.Marshal(entry)
		if err != nil {
			continue
		}

		subject := fmt.Sprintf("agentint.%s.events.%s", a.md.VmId, entry.Type())
		err = a.nc.Publish(subject, bytes)
		if err != nil {
			continue
		}

		a.nc.Flush()
	}
}

// This is run inside a goroutine to pull log entries off the channel and publish to the
// node host via internal NATS
func (a *Agent) dispatchLogs() {
	for {
		entry := <-a.agentLogs
		bytes, err := json.Marshal(entry)
		if err != nil {
			continue
		}

		subject := fmt.Sprintf("agentint.%s.logs", a.md.VmId)
		err = a.nc.Publish(subject, bytes)
		if err != nil {
			continue
		}

		a.nc.Flush()
	}
}
