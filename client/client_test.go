package client

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-labs/nex"
	"github.com/synadia-labs/nex/models"
)

const (
	Node1Seed = "SNAD2VTZQ7Z3CCB6TJWFFJK6J7DDZ5SQUHOFPQFRKDIVDQBM2WLGZGELSE"
	Node1Pub  = "NB4XOM2IOV2NRLRZZU5PFMQFBAQXYQOG4WOVGOWRYJRBWI3JO5QJK7AG"
)

func StartNatsServer(t testing.TB, workDir string) *server.Server {
	t.Helper()

	server := server.New(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  workDir,
	})

	server.Start()

	return server
}

// TODO - move the inmem nexlet here and use it for testing

func StartNexNode(t testing.TB, workDir string, nc *nats.Conn) *nex.NexNode {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	kp, err := nkeys.FromSeed([]byte(Node1Seed))
	be.NilErr(t, err)

	// runner, err := native.NewNativeWorkloadAgent(nc.ConnectedAddr(), Node1Pub, logger)
	// be.NilErr(t, err)

	node, err := nex.NewNexNode(
		nex.WithNodeKeyPair(kp),
		nex.WithNatsConn(nc),
		//		nex.WithAgentRunner(runner),
		nex.WithLogger(logger),
		nex.WithTag("foo", "bar"),
	)
	be.NilErr(t, err)

	be.NilErr(t, node.Start())
	return node
}

func TestNewNexClient(t *testing.T) {
	server := StartNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	client := NewClient(nc, "test")
	be.Nonzero(t, client)

	be.DeepEqual(t, nc, client.nc)
	be.Equal(t, "test", client.namespace)
}

func TestNexClient_User(t *testing.T) {
	t.SkipNow()
	workDir := t.TempDir()
	server := StartNatsServer(t, workDir)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	nn := StartNexNode(t, workDir, nc)
	defer nn.Shutdown() //nolint

	binPath := BuildWorkloadBinary(t, workDir)

	client := NewClient(nc, "user")
	be.Nonzero(t, client)

	ar, err := client.Auction("native", map[string]string{})
	be.NilErr(t, err)
	be.Equal(t, 1, len(ar))

	sr, err := client.StartWorkload(ar[0].BidderId, "tester", "My test workload", fmt.Sprintf(`{"uri":"%s"}`, binPath), "native", models.WorkloadLifecycleService)
	be.NilErr(t, err)
	be.Equal(t, "tester", sr.Name)

	logs := SampleWorkloadLogs(t, nc, time.Millisecond*1500, sr.Id)
	be.True(t, strings.Contains(logs, "Count:"))

	str, err := client.StopWorkload(sr.Id)
	be.NilErr(t, err)

	be.Equal(t, sr.Id, str.Id)
	be.True(t, str.Stopped)
}

func TestNexClient_System(t *testing.T) {
	workDir := t.TempDir()
	server := StartNatsServer(t, workDir)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	StartNexNode(t, workDir, nc)

	client := NewClient(nc, models.SystemNamespace)
	be.Nonzero(t, client)

	info, err := client.GetNodeInfo(Node1Pub)
	be.NilErr(t, err)
	be.Equal(t, Node1Pub, info.NodeId)

	nodes, err := client.ListNodes(map[string]string{"foo": "bar"})
	be.NilErr(t, err)
	be.Equal(t, 1, len(nodes))
	be.Equal(t, Node1Pub, nodes[0].NodeId)

	ldr, err := client.SetLameduck(Node1Pub, 0)
	be.NilErr(t, err)
	be.True(t, ldr.Success)
}

func TestNexClient_SystemAsUser(t *testing.T) {
	workDir := t.TempDir()
	server := StartNatsServer(t, workDir)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	StartNexNode(t, workDir, nc)

	client := NewClient(nc, "user")
	be.Nonzero(t, client)

	_, err = client.GetNodeInfo(Node1Pub)
	be.Equal(t, "node not found", err.Error())

	nodes, err := client.ListNodes(map[string]string{"foo": "bar"})
	be.NilErr(t, err)
	be.Equal(t, 0, len(nodes))

	_, err = client.SetLameduck(Node1Pub, 0)
	be.Equal(t, "no nodes found", err.Error())
}

func BuildWorkloadBinary(t testing.TB, workingDir string) string {
	t.Helper()

	const (
		main string = `package main

import (
    "fmt"
    "os"
    "os/signal"
    "time"
)

func main() {
    sigChan := make(chan os.Signal, 1)

    signal.Notify(sigChan, os.Interrupt)

    go func() {
        <-sigChan  
        os.Exit(0) 
    }()

    count := 0
    for {
        fmt.Fprintf(os.Stdout, "Count: %d\n", count)
        time.Sleep(1 * time.Second)
        count++
    }
}`
	)

	f, err := os.Create(filepath.Join(workingDir, "main.go"))
	be.NilErr(t, err)
	_, err = f.WriteString(main)
	be.NilErr(t, err)
	f.Close()

	out, err := exec.Command("go", "build", "-o", filepath.Join(workingDir, "workload"), filepath.Join(workingDir, "main.go")).CombinedOutput()
	if err != nil {
		t.Log(string(out))
	}
	be.NilErr(t, err)
	return filepath.Join(workingDir, "workload")
}

func SampleWorkloadLogs(t testing.TB, nc *nats.Conn, duration time.Duration, workloadId string) string {
	t.Helper()

	resp := strings.Builder{}
	sub, err := nc.Subscribe(models.AgentEmitLogSubject("user", workloadId), func(m *nats.Msg) {
		resp.Write(m.Data)
	})
	be.NilErr(t, err)

	time.Sleep(duration)
	err = sub.Unsubscribe()
	be.NilErr(t, err)

	return resp.String()
}
