package lib

import (
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
)

// Messaging operations available:
// Publish
// Request
// RequestMany

type MessagingService struct {
	log *slog.Logger
	nc  *nats.Conn
}

func NewMessagingService(nc *nats.Conn, log *slog.Logger) (*MessagingService, error) {
	messaging := &MessagingService{
		log: log,
		nc:  nc,
	}

	err := messaging.init()
	if err != nil {
		return nil, err
	}

	return messaging, nil
}

func (m *MessagingService) init() error {
	return nil
}

func (m *MessagingService) HandleRPC(msg *nats.Msg) {
	// agentint.{vmID}.rpc.{namespace}.{workload}.{service}.{method}
	tokens := strings.Split(msg.Subject, ".")
	service := tokens[5]
	method := tokens[6]

	switch method {
	default:
		m.log.Warn("Received invalid host services RPC request",
			slog.String("service", service),
			slog.String("method", method),
		)

		// msg.Respond()
	}
}
