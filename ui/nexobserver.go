package nexui

import (
	"encoding/json"
	"time"

	controlapi "github.com/ConnectEverything/nex/internal/control-api"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

type NexObserver struct {
	hub     *Hub
	conn    *nats.Conn
	nodeCtl *controlapi.Client
	log     *logrus.Logger
}

func newObserver(hub *Hub, nc *nats.Conn, log *logrus.Logger) (*NexObserver, error) {
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

func dispatchEvents(eventChannel chan controlapi.EmittedEvent, hub *Hub, log *logrus.Logger) {
	for {
		event := <-eventChannel
		log.WithField("event_type", event.EventType).Info("Dispatching event")
		eventBytes, err := json.Marshal(event)
		if err != nil {
			continue
		}
		hub.broadcastMessage(eventBytes)
	}
}

func dispatchLogs(logChannel chan controlapi.EmittedLog, hub *Hub, log *logrus.Logger) {
	for {
		logLine := <-logChannel
		log.WithField("namespace", logLine.Namespace).Info("Dispatching log line")
		logBytes, err := json.Marshal(logLine)
		if err != nil {
			continue
		}
		hub.broadcastMessage(logBytes)
	}
}
