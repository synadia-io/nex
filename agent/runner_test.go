package agent

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/carlmjohnson/be"
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

		nodeID:   "abc123",
		nexus:    "xyz890",
		agent:    nil,
		agentID:  "default",
		triggers: make(map[string]*triggerResources),
	}

	b, err := NewRunner(context.Background(), "xyz890", "abc123", nil)
	be.NilErr(t, err)

	be.DeepEqual(t, a, b)
}
