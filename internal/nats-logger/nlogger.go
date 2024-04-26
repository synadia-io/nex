package natslogger

import (
	"github.com/nats-io/nats.go"
)

type NatsLogger struct {
	nc       *nats.Conn
	OutTopic string
}

func NewNatsLogger(nc *nats.Conn, topic string) *NatsLogger {
	return &NatsLogger{
		OutTopic: topic,
		nc:       nc,
	}
}

func (nl *NatsLogger) Write(p []byte) (int, error) {
	err := nl.nc.Publish(nl.OutTopic, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
