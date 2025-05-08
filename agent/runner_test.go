package agent

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
)

func TestRunnerConstructor(t *testing.T) {
	a := &Runner{
		ctx:          context.Background(),
		name:         "default",
		registerType: "default",
		version:      "0.0.0",
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		metrics:      false,
		metricsPort:  9095,

		nodeId:   "abc123",
		agent:    nil,
		triggers: make(map[string]*nats.Subscription),
	}

	b, err := NewRunner(context.Background(), "abc123", nil)
	be.NilErr(t, err)

	be.DeepEqual(t, a, b)
}
