package lib

import (
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
)

// HTTP client operations available:
// Request (payload contains method, headers, etc)

type HTTPService struct {
	log *slog.Logger
	nc  *nats.Conn
}

func NewHTTPService(nc *nats.Conn, log *slog.Logger) (*HTTPService, error) {
	http := &HTTPService{
		log: log,
		nc:  nc,
	}

	err := http.init()
	if err != nil {
		return nil, err
	}

	return http, nil
}

func (h *HTTPService) init() error {
	return nil
}

func (h *HTTPService) HandleRPC(msg *nats.Msg) {
	// agentint.{vmID}.rpc.{namespace}.{workload}.{service}.{method}
	tokens := strings.Split(msg.Subject, ".")
	service := tokens[5]
	method := tokens[6]

	switch method {
	default:
		h.log.Warn("Received invalid host services RPC request",
			slog.String("service", service),
			slog.String("method", method),
		)

		// msg.Respond()
	}
}
