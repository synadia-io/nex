package node_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"

	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/nex/node"
)

func startNatsServer(t testing.TB) *nats.Conn {
	t.Helper()
	opts := &server.Options{
		Port: -1,
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatal("failed to start nats server", err)
	}
	defer ns.Shutdown()

	go ns.Start()
	time.Sleep(500 * time.Millisecond)

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatal("failed to connect to nats server", err)
	}

	return nc
}

func TestDefaultConfigNodeValidation(t *testing.T) {
	nc := startNatsServer(t)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	if err != nil {
		t.Fatal("failed to create keypair")
	}

	tDir := t.TempDir()

	node, err := node.NewNexNode(kp, nc, models.WithResourceDirectory(tDir), models.WithDisableDirectStart(true),
		models.WithExternalAgents([]models.AgentOptions{{Name: "test", Uri: "https://derp.com", Configuration: json.RawMessage{}}}))
	if err != nil {
		t.Fatal("failed to create new nex node", err)
	}
	if err := node.Validate(); err != nil {
		t.Fatal("failed to validate nex node", err)
	}
}

func TestNilOptions(t *testing.T) {
	nc := startNatsServer(t)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	if err != nil {
		t.Fatal("failed to create keypair")
	}

	_, err = node.NewNexNode(kp, nc, nil, models.WithDisableDirectStart(true))
	if err.Error() != `node required at least 1 workload type be configured in order to start
resource directory does not exist` {
		fmt.Printf("'%s'", err.Error())
		t.Fatal("failed to return expected errors")
	}
}

func TestNoOptions(t *testing.T) {
	nc := startNatsServer(t)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	if err != nil {
		t.Fatal("failed to create keypair")
	}

	_, err = node.NewNexNode(kp, nc, models.WithDisableDirectStart(true))
	if err.Error() != `node required at least 1 workload type be configured in order to start
resource directory does not exist` {
		fmt.Printf("'%s'", err.Error())
		t.Fatal("failed to return expected errors")
	}
}
