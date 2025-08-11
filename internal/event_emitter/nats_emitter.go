package eventemitter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/synadia-labs/nex/models"
)

var _ models.EventEmitter = (*NatsEmitter)(nil)

type NatsEmitter struct {
	ctx context.Context
	nc  *nats.Conn
}

func NewNatsEmitter(ctx context.Context, nc *nats.Conn) *NatsEmitter {
	return &NatsEmitter{
		ctx: ctx,
		nc:  nc,
	}
}

func (e *NatsEmitter) EmitEvent(from string, event any) error {
	eventBytes, err := toBytes(event)
	if err != nil {
		return fmt.Errorf("could not convert event to bytes: %w", err)
	}

	// Use the event's String() method to get the subject event suffix
	stringer, ok := event.(fmt.Stringer)
	if !ok {
		return fmt.Errorf("event type %T does not implement String()", event)
	}

	eventSubject := models.EventAPIPrefix(from) + "." + stringer.String()
	return e.nc.Publish(eventSubject, eventBytes)
}

func toBytes(e any) ([]byte, error) {
	switch v := e.(type) {
	case nil:
		return nil, nil

	case []byte:
		return v, nil

	case string:
		return []byte(v), nil

	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("bytesx: could not JSON-marshal %#v: %w", v, err)
		}
		return b, nil
	}
}
