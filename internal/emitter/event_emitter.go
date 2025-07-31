package emitter

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-labs/nex/models"
)

type NexNodeEvent interface {
	*models.NexNodeStartedEvent |
		*models.NexNodeStoppedEvent |
		*models.NexNodeLameduckSetEvent |
		*models.AgentStartedEvent |
		*models.AgentStoppedEvent |
		*models.AgentLameduckSetEvent
}

func EmitSystemEvent[T NexNodeEvent](nc *nats.Conn, kp nkeys.KeyPair, in T) error {
	if nc == nil || kp == nil {
		return fmt.Errorf("system event emitter requires a valid NATS connection and keypair")
	}

	pubKey, err := kp.PublicKey()
	if err != nil {
		return err
	}

	inB, err := json.Marshal(in)
	if err != nil {
		return err
	}

	return nc.Publish(fmt.Sprintf("%s.%s", models.EventAPIPrefix(pubKey), in), inB)
}
