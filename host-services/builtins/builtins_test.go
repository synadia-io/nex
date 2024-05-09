package builtins

import (
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	hostservices "github.com/synadia-io/nex/host-services"
)

const (
	testNamespace  = "testspace"
	testWorkload   = "testwork"
	testWorkloadId = "abc12346"
)

func TestKvBuiltin(t *testing.T) {
	nc, err := nats.Connect("0.0.0.0:4222")
	if err != nil {
		panic(err)
	}

	server := hostservices.NewHostServicesServer(nc)
	client := hostservices.NewHostServicesClient(nc, 2*time.Second, testNamespace, testWorkload, testWorkloadId)
	bClient := NewBuiltinServicesClient(client)

	service, _ := NewKeyValueService(nc, slog.Default())
	_ = server.AddService("keyvalue", service, make(map[string]string))

	_ = server.Start()

	_, err = bClient.KVSet("testone", []byte{9, 8, 7, 6, 5})
	if err != nil {
		t.Fatalf("Got an error setting kv: %s", err.Error())
	}

	v, err := bClient.KVGet("testone")
	if err != nil {
		t.Fatalf("Got an error getting key: %s", err.Error())
	}
	if !slices.Equal(v, []byte{9, 8, 7, 6, 5}) {
		t.Fatalf("Didn't get expected byte array back, got %+v", v)
	}

	keys, err := bClient.KVKeys()
	if err != nil {
		t.Fatalf("Failed to get bucket keys: %s", err.Error())
	}
	if !slices.Contains(keys, "testone") {
		t.Fatalf("Expected to see testone in the keys list, but got '%+v'", keys)
	}
}

func TestMessagingBuiltin(t *testing.T) {
	nc, err := nats.Connect("0.0.0.0:4222")
	if err != nil {
		panic(err)
	}

	server := hostservices.NewHostServicesServer(nc)
	client := hostservices.NewHostServicesClient(nc, 2*time.Second, testNamespace, testWorkload, testWorkloadId)
	bClient := NewBuiltinServicesClient(client)

	service, _ := NewMessagingService(nc, slog.Default())
	_ = server.AddService("messaging", service, make(map[string]string))
	_ = server.Start()

	wg := new(sync.WaitGroup)
	wg.Add(1)
	_, _ = nc.Subscribe("foo.bar", func(m *nats.Msg) {
		wg.Done()
	})

	err = bClient.MessagingPublish("foo.bar", []byte("baz"))
	if err != nil {
		t.Fatalf("Failed to publish message: %s", err)
	}

	wg.Wait()
}

func TestObjectBuiltin(t *testing.T) {
	nc, err := nats.Connect("0.0.0.0:4222")
	if err != nil {
		panic(err)
	}

	server := hostservices.NewHostServicesServer(nc)
	client := hostservices.NewHostServicesClient(nc, 2*time.Second, testNamespace, testWorkload, testWorkloadId)
	bClient := NewBuiltinServicesClient(client)

	service, _ := NewObjectStoreService(nc, slog.Default())
	_ = server.AddService("objectstore", service, make(map[string]string))
	_ = server.Start()

	res, err := bClient.ObjectPut("objecttest", []byte{100, 101, 102})
	if err != nil {
		t.Fatalf("Expected no error, but got %s", err.Error())
	}
	if res.Name != "objecttest" {
		t.Fatalf("Incorrectly named object, got %s", res.Name)
	}
	if res.Size != 3 {
		t.Fatalf("Expected to store 3 bytes, got %d", res.Size)
	}

	res2, err := bClient.ObjectGet("objecttest")
	if err != nil {
		t.Fatalf("Failed to retrieve object: %s", err.Error())
	}
	if !slices.Equal(res2, []byte{100, 101, 102}) {
		t.Fatalf("Retrieved the wrong bytes, got %v", res2)
	}

	infos, err := bClient.ObjectList()
	if err != nil {
		t.Fatalf("Failed to list objects in bucket")
	}
	if infos[0].Name != "objecttest" {
		t.Fatalf("Got unexpected list of items in bucket: %+v", infos)
	}

	err = bClient.ObjectDelete("objecttest")
	if err != nil {
		t.Fatalf("Failed to delete object: %s", err.Error())
	}

	res3, err := bClient.ObjectGet("objecttest")
	if err == nil {
		t.Fatalf("Expected to get an error for non-existing object but didn't: %+v", res3)
	}
}
