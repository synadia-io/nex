package lib

import (
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
)

// Object store operations available:
// PutChunk
// GetChunk
// Delete
// Keys (this can fail beyond some upper limit and/or require some form of paging)

type ObjectStoreService struct {
	log *slog.Logger
	nc  *nats.Conn
}

func NewObjectStoreService(nc *nats.Conn, log *slog.Logger) (*ObjectStoreService, error) {
	objectStore := &ObjectStoreService{
		log: log,
		nc:  nc,
	}

	err := objectStore.init()
	if err != nil {
		return nil, err
	}

	return objectStore, nil
}

func (o *ObjectStoreService) init() error {
	return nil
}

func (o *ObjectStoreService) HandleRPC(msg *nats.Msg) {
	// agentint.{vmID}.rpc.{namespace}.{workload}.{service}.{method}
	tokens := strings.Split(msg.Subject, ".")
	service := tokens[5]
	method := tokens[6]

	switch method {
	default:
		o.log.Warn("Received invalid host services RPC request",
			slog.String("service", service),
			slog.String("method", method),
		)

		// msg.Respond()
	}
}
