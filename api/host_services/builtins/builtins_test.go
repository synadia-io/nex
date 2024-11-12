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
	"github.com/nats-io/nats.go/jetstream"
	hostservices "github.com/synadia-io/nex/api/host_services"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	testNamespace     = "testspace"
	testWorkloadType  = "amazeballs"
	testWorkload      = "testwork"
	testWorkloadId    = "abc12346"
	testBucketName    = "testBucket"
	testObjBucketName = "testObjBucket"
)

func setupSuite(t testing.TB, port int) (*nats.Conn, func(tb testing.TB)) {
	svr, _ := server.NewServer(&server.Options{
		Port:      port,
		Host:      "0.0.0.0",
		JetStream: true,
	})
	svr.Start()
	ctx := context.Background()

	nc, _ := nats.Connect(svr.ClientURL())
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  testBucketName,
		Storage: jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{
		Bucket:  testObjBucketName,
		Storage: jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Return a function to teardown the test
	return nc, func(tb testing.TB) {
		svr.Shutdown()
		if js != nil {
			_ = js.DeleteKeyValue(ctx, testBucketName)
			_ = js.DeleteObjectStore(ctx, testObjBucketName)
		}
	}
}

func TestKvBuiltin(t *testing.T) {
	nc, teardownSuite := setupSuite(t, 4446)
	defer teardownSuite(t)

	server := hostservices.NewHostServicesServer(nc, slog.Default(), noop.NewTracerProvider().Tracer("nex-node"))
	client := hostservices.NewHostServicesClient(nc, 2*time.Second, testNamespace, testWorkloadId)
	bClient := NewBuiltinServicesClient(client, testWorkloadType)
	server.SetHostServicesConnection(testWorkloadId, nc)

	service, _ := NewKeyValueService(slog.Default())
	err := server.AddService("kv", service, nil)
	if err != nil {
		t.Fatalf("Failed to add service: %s", err)
	}

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %s", err)
	}

	_, err = bClient.KVSet(context.Background(), testBucketName, "testone", []byte{9, 8, 7, 6, 5})
	if err != nil {
		t.Fatalf("Got an error setting kv: %s", err.Error())
	}

	v, err := bClient.KVGet(context.Background(), testBucketName, "testone")
	if err != nil {
		t.Fatalf("Got an error getting key: %s", err.Error())
	}
	if !slices.Equal(v, []byte{9, 8, 7, 6, 5}) {
		t.Fatalf("Didn't get expected byte array back, got %+v", v)
	}

	keys, err := bClient.KVKeys(context.Background(), testBucketName)
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

	server := hostservices.NewHostServicesServer(nc, slog.Default(), noop.NewTracerProvider().Tracer("nex-node"))
	client := hostservices.NewHostServicesClient(nc, 2*time.Second, testNamespace, testWorkloadId)
	bClient := NewBuiltinServicesClient(client, testWorkloadType)
	server.SetHostServicesConnection(testWorkloadId, nc)

	service, _ := NewMessagingService(slog.Default())
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

	server := hostservices.NewHostServicesServer(nc, slog.Default(), noop.NewTracerProvider().Tracer("nex-node"))
	client := hostservices.NewHostServicesClient(nc, 2*time.Second, testNamespace, testWorkloadId)
	bClient := NewBuiltinServicesClient(client, testWorkloadType)
	server.SetHostServicesConnection(testWorkloadId, nc)

	service, _ := NewObjectStoreService(slog.Default())
	msgService, _ := NewMessagingService(slog.Default())
	_ = server.AddService("objectstore", service, []byte{})
	_ = server.AddService("messaging", msgService, []byte{})
	_ = server.Start()

	res, err := bClient.ObjectPut(context.Background(), testObjBucketName, "objecttest", []byte{100, 101, 102})
	if err != nil {
		t.Fatalf("Expected no error, but got %s", err.Error())
	}
	if res.Name != "objecttest" {
		t.Fatalf("Incorrectly named object, got %s", res.Name)
	}
	if res.Size != 3 {
		t.Fatalf("Expected to store 3 bytes, got %d", res.Size)
	}

	res2, err := bClient.ObjectGet(context.Background(), testObjBucketName, "objecttest")
	if err != nil {
		t.Fatalf("Failed to retrieve object: %s", err.Error())
	}
	if !slices.Equal(res2, []byte{100, 101, 102}) {
		t.Fatalf("Retrieved the wrong bytes, got %v", res2)
	}

	infos, err := bClient.ObjectList(context.Background(), testObjBucketName)
	if err != nil {
		t.Fatalf("Failed to list objects in bucket")
	}
	if infos[0].Name != "objecttest" {
		t.Fatalf("Got unexpected list of items in bucket: %+v", infos)
	}

	err = bClient.ObjectDelete(context.Background(), testObjBucketName, "objecttest")
	if err != nil {
		t.Fatalf("Failed to delete object: %s", err.Error())
	}

	res3, err := bClient.ObjectGet(context.Background(), testObjBucketName, "objecttest")
	if err == nil {
		t.Fatalf("Expected to get an error for non-existing object but didn't: %+v", res3)
	}

	err = bClient.MessagingPublish(context.Background(), "foo", []byte{1, 2, 3})
	if err != nil {
		t.Fatalf("Failed to use trigger connection for messaging publish: %s", err.Error())
	}
}
