package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func startNatsServer(t testing.TB) (*server.Server, error) {
	t.Helper()

	s, err := server.NewServer(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	s.Start()
	time.Sleep(500 * time.Millisecond)

	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("failed to create jetstream context: %v", err)
	}
	_, err = js.CreateKeyValue(context.TODO(), jetstream.KeyValueConfig{
		Bucket: "bucket",
	})
	if err != nil {
		t.Fatalf("failed to create kv: %v", err)
	}
	_ = nc.Drain()

	return s, nil
}

func TestLogSubject(t *testing.T) {
	natsServer, err := startNatsServer(t)
	if err != nil {
		t.Fatalf("failed to start nats server: %v", err)
	}
	defer natsServer.Shutdown()
	nc, err := nats.Connect(natsServer.ClientURL())
	if err != nil {
		t.Fatalf("failed to connect to server: %v", err)
	}

	m := Monitor{
		Logs: Logs{
			WorkloadID: "*",
			Level:      "*",
		},
	}

	done := make(chan struct{}, 1)
	origStdout := os.Stdout
	stdout := new(bytes.Buffer)
	r, w, _ := os.Pipe()
	os.Stdout = w

	go func() {
		_, _ = io.Copy(stdout, r)
		<-done
	}()

	globals := &Globals{}
	globals.NatsServers = []string{natsServer.ClientURL()}
	globals.Namespace = "system"

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err = m.Logs.Run(ctx, globals)
		if err != nil {
			t.Log(stdout.String())
			t.Errorf("failed to run logs: %v", err)
		}
	}()

	time.Sleep(1 * time.Second)

	err = nc.Publish("$NEX.logs.a.b", []byte("derp"))
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	done <- struct{}{}
	time.Sleep(200 * time.Millisecond)
	w.Close()
	os.Stdout = origStdout
	cancel()

	if strings.Contains(stdout.String(), "$NEX.logs.a.b [1.0s] -> derp\n") {
		t.Errorf("unexpected output: '%s'", stdout.String())
	}
}
