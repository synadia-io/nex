package builtins

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	hostservices "github.com/synadia-io/nex/host-services"
)

const (
	testNamespace  = "testspace"
	testWorkload   = "testwork"
	testWorkloadId = "abc12346"
)

func setupSuite(_ testing.TB, port int) (*nats.Conn, func(tb testing.TB)) {
	svr, _ := server.NewServer(&server.Options{
		Port:      port,
		Host:      "0.0.0.0",
		JetStream: true,
	})
	svr.Start()

	nc, _ := nats.Connect(svr.ClientURL())

	// Return a function to teardown the test
	return nc, func(tb testing.TB) {
		svr.Shutdown()
	}
}

func TestKvBuiltin(t *testing.T) {
	nc, teardownSuite := setupSuite(t, 4446)
	defer teardownSuite(t)

	server := hostservices.NewHostServicesServer(nc, slog.Default(), nil)
	client := hostservices.NewHostServicesClient(nc, 2*time.Second, testNamespace, testWorkload, testWorkloadId)
	bClient := NewBuiltinServicesClient(client)

	service, _ := NewKeyValueService(nc, slog.Default())
	err := server.AddService("kv", service, nil)
	if err != nil {
		t.Fatalf("Failed to add service: %s", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %s", err)
	}

	_, err = bClient.KVSet(context.Background(), "testone", []byte{9, 8, 7, 6, 5})
	if err != nil {
		t.Fatalf("Got an error setting kv: %s", err.Error())
	}

	v, err := bClient.KVGet(context.Background(), "testone")
	if err != nil {
		t.Fatalf("Got an error getting key: %s", err.Error())
	}
	if !slices.Equal(v, []byte{9, 8, 7, 6, 5}) {
		t.Fatalf("Didn't get expected byte array back, got %+v", v)
	}

	keys, err := bClient.KVKeys(context.Background())
	if err != nil {
		t.Fatalf("Failed to get bucket keys: %s", err.Error())
	}
	if !slices.Contains(keys, "testone") {
		t.Fatalf("Expected to see testone in the keys list, but got '%+v'", keys)
	}
}

func TestMessagingBuiltin(t *testing.T) {
	nc, teardownSuite := setupSuite(t, 4447)
	defer teardownSuite(t)

	server := hostservices.NewHostServicesServer(nc, slog.Default(), nil)
	client := hostservices.NewHostServicesClient(nc, 2*time.Second, testNamespace, testWorkload, testWorkloadId)
	bClient := NewBuiltinServicesClient(client)

	service, _ := NewMessagingService(nc, slog.Default())
	_ = server.AddService("messaging", service, nil)
	_ = server.Start()

	wg := new(sync.WaitGroup)
	wg.Add(1)
	_, _ = nc.Subscribe("foo.bar", func(m *nats.Msg) {
		wg.Done()
	})

	err := bClient.MessagingPublish(context.Background(), "foo.bar", []byte("baz"))
	if err != nil {
		t.Fatalf("Failed to publish message: %s", err)
	}

	wg.Wait()
}

func TestObjectBuiltin(t *testing.T) {
	nc, teardownSuite := setupSuite(t, 4448)
	defer teardownSuite(t)

	server := hostservices.NewHostServicesServer(nc, slog.Default(), nil)
	client := hostservices.NewHostServicesClient(nc, 2*time.Second, testNamespace, testWorkload, testWorkloadId)
	bClient := NewBuiltinServicesClient(client)

	service, _ := NewObjectStoreService(nc, slog.Default())
	_ = server.AddService("objectstore", service, []byte{})
	_ = server.Start()

	res, err := bClient.ObjectPut(context.Background(), "objecttest", []byte{100, 101, 102})
	if err != nil {
		t.Fatalf("Expected no error, but got %s", err.Error())
	}
	if res.Name != "objecttest" {
		t.Fatalf("Incorrectly named object, got %s", res.Name)
	}
	if res.Size != 3 {
		t.Fatalf("Expected to store 3 bytes, got %d", res.Size)
	}

	res2, err := bClient.ObjectGet(context.Background(), "objecttest")
	if err != nil {
		t.Fatalf("Failed to retrieve object: %s", err.Error())
	}
	if !slices.Equal(res2, []byte{100, 101, 102}) {
		t.Fatalf("Retrieved the wrong bytes, got %v", res2)
	}

	infos, err := bClient.ObjectList(context.Background())
	if err != nil {
		t.Fatalf("Failed to list objects in bucket")
	}
	if infos[0].Name != "objecttest" {
		t.Fatalf("Got unexpected list of items in bucket: %+v", infos)
	}

	err = bClient.ObjectDelete(context.Background(), "objecttest")
	if err != nil {
		t.Fatalf("Failed to delete object: %s", err.Error())
	}

	res3, err := bClient.ObjectGet(context.Background(), "objecttest")
	if err == nil {
		t.Fatalf("Expected to get an error for non-existing object but didn't: %+v", res3)
	}
}
