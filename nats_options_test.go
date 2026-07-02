package nex

import (
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/models"
)

// TestNatsConnectionOptions verifies that the node's NATS connection options
// keep the client reconnecting through auth errors (e.g. an expired/rotated
// JWT) instead of permanently aborting the reconnect loop.
func TestNatsConnectionOptions(t *testing.T) {
	opts := natsConnectionOptions(&models.NatsConnectionData{
		ConnName:     "test",
		NatsUserJwt:  "jwt",
		NatsUserSeed: "seed",
	})

	// Each nats.Option is a func(*nats.Options) error; fold them into a fresh
	// Options struct to inspect the resulting configuration without opening a
	// real NATS connection.
	o := &nats.Options{}
	for _, opt := range opts {
		be.NilErr(t, opt(o))
	}

	be.True(t, o.IgnoreAuthErrorAbort)
	be.Equal(t, -1, o.MaxReconnect)
}
