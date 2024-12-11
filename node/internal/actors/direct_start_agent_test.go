package actors

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/tochemey/goakt/v2/log"
	"github.com/tochemey/goakt/v2/testkit"

	"github.com/synadia-io/nex/models"
	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
)

func testNatsServer(t testing.TB, workingDir string) (*server.Server, error) {
	t.Helper()

	s := server.New(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  workingDir,
	})

	s.Start()

	go s.WaitForShutdown()

	return s, nil
}

func TestPingAgent(t *testing.T) {
	s, err := testNatsServer(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	xkey, err := nkeys.CreateCurveKeys()
	if err != nil {
		t.Fatal(err)
	}

	stdout := new(bytes.Buffer)
	logger := slog.New(slog.NewTextHandler(stdout, nil))

	ctx := context.Background()
	tk := testkit.New(ctx, t, testkit.WithLogging(log.ErrorLevel))
	tk.Spawn(ctx, DirectStartActorName, CreateDirectStartAgent(nc, "testnode", models.NodeOptions{Logger: logger, Xkey: xkey}, logger))

	probe := tk.NewProbe(ctx)

	probe.SendSync(DirectStartActorName, &actorproto.PingAgent{Namespace: "system"}, time.Second*3)
	respEnv, ok := probe.ExpectAnyMessage().(*actorproto.Envelope)
	if !ok {
		t.Fatalf("unexpected message type: %T", respEnv)
	}

	var resp actorproto.PingAgentResponse
	err = respEnv.Payload.UnmarshalTo(&resp)
	if err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if len(resp.GetRunningWorkloads()) != 0 {
		t.Fatalf("unexpected running workloads: %v", resp.GetRunningWorkloads())
	}

	tk.Shutdown(ctx)
}
