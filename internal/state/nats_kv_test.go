package state

import (
	"context"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/synadia-io/nex/models"
)

func startNatsServer(t testing.TB, workDir string) *server.Server {
	t.Helper()

	server := server.New(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  workDir,
	})

	server.Start()

	return server
}

func getChanCount(t testing.TB, ch <-chan string) int {
	t.Helper()

	count := 0
	for range ch {
		count++
	}

	return count
}

func TestNewKVState(t *testing.T) {
	server := startNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)

	s, err := NewNatsKVState(nc, "test", nil)
	be.NilErr(t, err)

	jsCtx, err := jetstream.New(nc)
	be.NilErr(t, err)

	kv, err := jsCtx.KeyValue(context.TODO(), "test")
	be.NilErr(t, err)

	kl, err := kv.ListKeys(context.TODO())
	be.NilErr(t, err)
	be.Equal(t, 0, getChanCount(t, kl.Keys()))

	be.NilErr(t, s.StoreWorkload("workload1", models.StartWorkloadRequest{
		Description:       "foogoo",
		Name:              "foogoo",
		Namespace:         "goo",
		RunRequest:        "{}",
		WorkloadLifecycle: "service",
		WorkloadType:      "foo",
	}))
	kl, err = kv.ListKeys(context.TODO())
	be.NilErr(t, err)
	be.Equal(t, 1, getChanCount(t, kl.Keys()))

	be.NilErr(t, s.StoreWorkload("workload2", models.StartWorkloadRequest{
		Description:       "goo",
		Name:              "goo",
		Namespace:         "goo",
		RunRequest:        "{}",
		WorkloadLifecycle: "service",
		WorkloadType:      "bar",
	}))
	kl, err = kv.ListKeys(context.TODO())
	be.NilErr(t, err)
	be.Equal(t, 2, getChanCount(t, kl.Keys()))

	fooState, err := s.GetStateByAgent("foo")
	be.NilErr(t, err)
	be.Equal(t, 1, len(fooState))

	gooState, err := s.GetStateByNamespace("goo")
	be.NilErr(t, err)
	be.Equal(t, 2, len(gooState))

	be.NilErr(t, s.RemoveWorkload("foo", "workload1"))
	kl, err = kv.ListKeys(context.TODO())
	be.NilErr(t, err)
	be.Equal(t, 1, getChanCount(t, kl.Keys()))

	be.NilErr(t, s.RemoveWorkload("bar", "workload2"))
	kl, err = kv.ListKeys(context.TODO())
	be.NilErr(t, err)
	be.Equal(t, 0, getChanCount(t, kl.Keys()))
}
