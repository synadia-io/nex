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

func NewNatsEmitter(ctx context.Context, nc *nats.Conn, subject string) *NatsEmitter {
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

	switch event.(type) {
	case *models.WorkloadStartedEvent:
		return e.nc.Publish(models.EventAPIPrefix(from)+".WORKLOADSTARTED", eventBytes)
	case *models.WorkloadStoppedEvent:
		return e.nc.Publish(models.EventAPIPrefix(from)+".WORKLOADSTOPPED", eventBytes)
	case *models.NexNodeStartedEvent:
		return e.nc.Publish(models.EventAPIPrefix(from)+".NODESTARTED", eventBytes)
	case *models.NexNodeStoppedEvent:
		return e.nc.Publish(models.EventAPIPrefix(from)+".NODESTOPPED", eventBytes)
	case *models.NexNodeLameduckSetEvent:
		return e.nc.Publish(models.EventAPIPrefix(from)+".NODELAMEDUCKSET", eventBytes)
	case *models.AgentStartedEvent:
		return e.nc.Publish(models.EventAPIPrefix(from)+".AGENTSTARTED", eventBytes)
	case *models.AgentStoppedEvent:
		return e.nc.Publish(models.EventAPIPrefix(from)+".AGENTSTOPPED", eventBytes)
	case *models.AgentLameduckSetEvent:
		return e.nc.Publish(models.EventAPIPrefix(from)+".AGENTLAMEDUCKSET", eventBytes)
	default:
		return fmt.Errorf("unsupported event type: %T", event)
	}
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
