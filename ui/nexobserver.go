package nexui

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	controlapi "github.com/synadia-io/nex/internal/control-api"
)

type NexObserver struct {
	hub     *Hub
	conn    *nats.Conn
	nodeCtl *controlapi.Client
	log     *slog.Logger
}

func newObserver(hub *Hub, nc *nats.Conn, log *slog.Logger) (*NexObserver, error) {
	return &NexObserver{
		hub:     hub,
		conn:    nc,
		nodeCtl: controlapi.NewApiClient(nc, 750*time.Millisecond, log),
		log:     log,
	}, nil
}

func (observer *NexObserver) run() error {
	eventChannel, err := observer.nodeCtl.MonitorAllEvents()
	if err != nil {
		return err
	}

	logChannel, err := observer.nodeCtl.MonitorAllLogs()
	if err != nil {
		return err
	}

	observer.log.Info("Starting dispatchers for events and logs")
	go dispatchEvents(eventChannel, observer.hub, observer.log)
	go dispatchLogs(logChannel, observer.hub, observer.log)

	return nil
}

func dispatchEvents(eventChannel chan controlapi.EmittedEvent, hub *Hub, log *slog.Logger) {
	for {
		event := <-eventChannel
		log.Info("Dispatching event", "event_type", event.EventType)
		eventBytes, err := json.Marshal(event)
		if err != nil {
			log.Error("failed to marchal event bytes", err)
			continue
		}
		hub.broadcastMessage(eventBytes)
	}
}

func dispatchLogs(logChannel chan controlapi.EmittedLog, hub *Hub, log *slog.Logger) {
	for {
		logLine := <-logChannel
		log.Info("Dispatching log line", "namespace", logLine.Namespace)
		logBytes, err := json.Marshal(logLine)
		if err != nil {
			log.Error("failed to marchal log bytes", err)
			continue
		}
		hub.broadcastMessage(logBytes)
	}
}
