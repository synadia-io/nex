package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"github.com/nats-io/nkeys"
	"github.com/synadia-labs/nex/_test"
	inmem "github.com/synadia-labs/nex/_test/nexlet_inmem"
	"github.com/synadia-labs/nex/models"
	"github.com/synadia-labs/nex/sdk/go/agent"
)

func TestRunningRunner(t *testing.T) {
	t.Helper()

	const (
		NexusName  = "testnexus"
		NexletName = "inmem"
	)

	ctx, cancel := context.WithTimeoutCause(context.Background(), time.Second*5, fmt.Errorf("test timed out"))
	defer cancel()

	s := _test.StartNatsServer(t, t.TempDir())
	defer s.Shutdown()

	kp, err := nkeys.CreateCurveKeys()
	be.NilErr(t, err)

	inmemNexlet := &inmem.InMemAgent{
		Name:         NexletName,
		Nexus:        NexusName,
		WorkloadType: NexletName,
		Version:      "0.0.0-test",
		Workloads: inmem.Workloads{
			State: make(map[string][]inmem.InMemWorkload),
		},
		XPair:     kp,
		StartTime: time.Now(),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	runner, err := agent.NewRunner(context.Background(), NexusName, _test.Node1Pub, inmemNexlet)
	be.NilErr(t, err)

	inmemNexlet.Runner = runner

	nn := _test.StartNexus(t, ctx, s.ClientURL(), 1, false, runner)
	be.Equal(t, 1, len(nn))

	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		for range ticker.C {
			if !runner.ServiceIsRunning() {
				ticker.Stop()
				cancel()
			}
		}
	}()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)

	resp, err := nc.Request(models.GetAgentIdByNameSubject(_test.Node1Pub), []byte(NexletName), time.Second*5)
	be.NilErr(t, err)
	agentId := resp.Data
	be.Nonzero(t, agentId)

	// This is the actual test
	go func() {
		t.Run("RunnerRunning", func(t *testing.T) {
			be.True(t, runner.ServiceIsRunning())
		})

		t.Run("StopWorkloadDNE", func(t *testing.T) {
			swr := models.StopWorkloadRequest{
				Namespace: "default",
			}
			swr_b, err := json.Marshal(swr)
			be.NilErr(t, err)

			resp, err := nc.Request(
				models.AgentAPIStopWorkloadRequestSubject(_test.Node1Pub, "abc123"),
				swr_b,
				time.Second*5,
			)
			be.NilErr(t, err)

			var swresp models.StopWorkloadResponse
			err = json.Unmarshal(resp.Data, &swresp)
			be.NilErr(t, err)

			be.Equal(t, "100", resp.Header.Get(micro.ErrorCodeHeader))
			be.Equal(t, "workload not found", swresp.Message)
			be.Equal(t, string(models.GenericErrorsNamespaceNotFound), resp.Header.Get(micro.ErrorHeader))

			be.False(t, swresp.Stopped)
		})

		t.Run("StartWorkload", func(t *testing.T) {
			swr := models.AgentStartWorkloadRequest{
				Request: models.StartWorkloadRequest{
					Description:       "test",
					Name:              "test",
					Namespace:         "default",
					RunRequest:        "{}",
					WorkloadLifecycle: models.WorkloadLifecycleService,
					WorkloadType:      NexletName,
				},
				WorkloadCreds: models.NatsConnectionData{
					NatsServers: []string{s.ClientURL()},
				},
			}
			swr_b, err := json.Marshal(swr)
			be.NilErr(t, err)

			resp, err := nc.Request(
				models.AgentAPIStartWorkloadRequestSubject(_test.Node1Pub, string(resp.Data), "abc123"),
				swr_b,
				time.Second*5,
			)
			be.NilErr(t, err)

			var swresp models.StartWorkloadResponse
			err = json.Unmarshal(resp.Data, &swresp)
			be.NilErr(t, err)

			be.Equal(t, "abc123", swresp.Id)
			be.Equal(t, "test", swresp.Name)
			be.Zero(t, resp.Header.Get(micro.ErrorCodeHeader))
		})

		t.Run("StopWorkload", func(t *testing.T) {
			swr := models.StopWorkloadRequest{
				Namespace: "default",
			}
			swr_b, err := json.Marshal(swr)
			be.NilErr(t, err)

			resp, err := nc.Request(
				models.AgentAPIStopWorkloadRequestSubject(_test.Node1Pub, "abc123"),
				swr_b,
				time.Second*5,
			)
			be.NilErr(t, err)

			var swresp models.StopWorkloadResponse
			err = json.Unmarshal(resp.Data, &swresp)
			be.NilErr(t, err)

			be.True(t, swresp.Stopped)
			be.Zero(t, swresp.Message)
			be.Zero(t, resp.Header.Get(micro.ErrorCodeHeader))
			be.Zero(t, resp.Header.Get(micro.ErrorHeader))
		})

		t.Run("RunnerShutdown", func(t *testing.T) {
			be.NilErr(t, runner.Shutdown())
		})
	}()

	<-ctx.Done()
	for _, n := range nn {
		be.NilErr(t, n.Shutdown())
	}
}
