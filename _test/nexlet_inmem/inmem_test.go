package inmem

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/sdk/go/agent"
	"github.com/synadia-io/nex"
	testminter "github.com/synadia-io/nex/_test/minter"
	"github.com/synadia-io/nex/client"
	"github.com/synadia-io/nex/models"
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
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestOptions(t *testing.T) {
	inmem, err := newInMemAgent("nexus", "PUBKEY", slog.New(slog.NewTextHandler(io.Discard, nil)), WithAgentName("foo"), WithWorkloadType("bar"))
	be.NilErr(t, err)
	be.Equal(t, "foo", inmem.Name)
	be.Equal(t, "bar", inmem.WorkloadType)
}

func TestStopWorkload(t *testing.T) {
	agt := newInmemAgent(t, true)
	agt.Runner = &agent.Runner{
		EmitEvent: func(string, any) error { return nil },
	}

	err := agt.StopWorkload("abc123", &models.StopWorkloadRequest{
		Namespace: "default",
	})
	be.NilErr(t, err)

	// Check that the workload is removed from the state
	_, ok := agt.Workloads.State["test"]
	be.False(t, ok)

	// Check that memory is released
	be.Equal(t, 0, len(agt.Workloads.State))
}

func TestStartWorkload(t *testing.T) {
	agt := newInmemAgent(t, false)
	agt.Runner = &agent.Runner{
		EmitEvent: func(string, any) error { return nil },
	}

	swr, err := agt.StartWorkload("abc123", &models.AgentStartWorkloadRequest{
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

	workloads := agt.Workloads.State["default"]
	be.Equal(t, 1, len(workloads))
	be.Equal(t, "tester", workloads[0].name)
}

func TestFunctionStartWorkloadAndTrigger(t *testing.T) {
	server := server.New(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	})
	server.Start()
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	nodePub, err := kp.PublicKey()
	be.NilErr(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	runner, err := NewInMemAgent("nexus", nodePub, logger)
	be.NilErr(t, err)

	nn, err := nex.NewNexNode(
		nex.WithContext(t.Context()),
		nex.WithNexus("nexus"),
		nex.WithNodeKeyPair(kp),
		nex.WithAgentRunner(runner),
		nex.WithNatsConn(nc),
		nex.WithLogger(logger),
		nex.WithMinter(&testminter.TestMinter{NatsServers: []string{nc.ConnectedUrl()}}),
	)
	be.NilErr(t, err)
	be.NilErr(t, nn.Start())
	for nn.IsReady() {
		break
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*3)
	defer cancel()

	nClient, err := client.NewClient(ctx, nc, models.SystemNamespace)
	be.NilErr(t, err)
	be.Nonzero(t, nClient)

	aR, err := nClient.Auction("inmem", nil)
	be.NilErr(t, err)
	be.Equal(t, 1, len(aR))

	sR, err := nClient.StartWorkload(aR[0].BidderId, "inmemfunc", "", "{}", "inmem", "function", nil)
	be.NilErr(t, err)
	be.Nonzero(t, sR)

	msg, err := nc.Request(sR.Id, []byte("test message"), time.Second)
	be.NilErr(t, err)
	be.Equal(t, "Function executed successfully", string(msg.Data))

	be.NilErr(t, nn.Shutdown())
}
