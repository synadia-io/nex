package agent

import (
	"testing"

	"github.com/carlmjohnson/be"
)

func TestRunnerConstructor(t *testing.T) {
	ctx := t.Context()
	a := &Runner{
		ctx:          ctx,
		name:         "default",
		registerType: "default",
		version:      "0.0.0",
		logger:       defaultLogger,
		metrics:      false,
		metricsPort:  9095,

		nodeID:   "abc123",
		nexus:    "xyz890",
		agent:    nil,
		agentID:  "default",
		triggers: make(map[string]*triggerResources),

		secretStore: nil,
		EmitEvent:   defaultEmitEvent,
	}

	b, err := NewRunner(ctx, "xyz890", "abc123", nil)
	be.NilErr(t, err)

	be.Equal(t, a.ctx, b.ctx)
	be.Equal(t, a.name, b.name)
	be.Equal(t, a.registerType, b.registerType)
	be.Equal(t, a.version, b.version)
	be.Equal(t, a.logger, b.logger)
	be.Equal(t, a.metrics, b.metrics)
	be.Equal(t, a.metricsPort, b.metricsPort)
	be.Equal(t, a.nodeID, b.nodeID)
	be.Equal(t, a.nexus, b.nexus)
	be.Equal(t, a.agentID, b.agentID)
	be.DeepEqual(t, a.triggers, b.triggers)
	be.Equal(t, a.secretStore, b.secretStore)
}
