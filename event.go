package nex

import (
	"encoding/json"

	"github.com/synadia-labs/nex/models"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

type NexNodeEvent interface {
	*models.NexNodeStartedEvent |
		*models.AgentStartedEvent |
		*models.AgentLameduckSetEvent
}

func emitSystemEvent[T NexNodeEvent](nc *nats.Conn, kp nkeys.KeyPair, in T) error {
	pubKey, err := kp.PublicKey()
	if err != nil {
		return err
	}

	inB, err := json.Marshal(in)
	if err != nil {
		return err
	}

	return nc.Publish(models.EmitSystemEventSubject(pubKey), inB)
}
