package inmem

import (
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nkeys"
	"github.com/synadia-labs/nex/models"
)

func newInmemAgent(t testing.TB, withState bool) *InMemAgent {
	t.Helper()

	kp, err := nkeys.CreateCurveKeys()
	be.NilErr(t, err)

	state := make(map[string][]InMemWorkload)
	if withState {
		state["default"] = []InMemWorkload{
			{
				id:        "abc123",
				name:      "tester",
				startTime: time.Now(),
				startRequest: &models.StartWorkloadRequest{
					Description:       "tester",
					Name:              "tester",
					Namespace:         "default",
					RunRequest:        "{}",
					WorkloadLifecycle: "service",
					WorkloadType:      "inmem",
				},
			},
		}
	}

	return &InMemAgent{
		Name:    "inmem",
		Version: "0.0.0-test",
		Workloads: Workloads{
			State: state,
		},
		XPair:     kp,
		StartTime: time.Now(),
	}
}

func TestStopWorkload(t *testing.T) {
	agent := newInmemAgent(t, true)

	err := agent.StopWorkload("abc123", &models.StopWorkloadRequest{
		Namespace: "default",
	})
	be.NilErr(t, err)

	// Check that the workload is removed from the state
	_, ok := agent.Workloads.State["test"]
	be.False(t, ok)

	// Check that memory is released
	be.Equal(t, 0, len(agent.Workloads.State))
}

func TestStartWorkload(t *testing.T) {
	agent := newInmemAgent(t, false)

	swr, err := agent.StartWorkload("abc123", &models.AgentStartWorkloadRequest{
		Request: models.StartWorkloadRequest{
			Description:       "tester",
			Name:              "tester",
			Namespace:         "default",
			RunRequest:        "{}",
			WorkloadLifecycle: "service",
			WorkloadType:      "inmem",
		},
		WorkloadCreds: models.NatsConnectionData{},
	}, false)
	be.NilErr(t, err)

	be.Equal(t, "abc123", swr.Id)
	be.Equal(t, "tester", swr.Name)

	workloads := agent.Workloads.State["default"]
	be.Equal(t, 1, len(workloads))
	be.Equal(t, "tester", workloads[0].name)
}
