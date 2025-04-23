package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/carlmjohnson/be"
	"github.com/nats-io/nkeys"
	"github.com/synadia-labs/nex/_test"
	"github.com/synadia-labs/nex/models"
)

// func startNexus(t testing.TB, ctx context.Context, natsUrl string, size int) {
// 	t.Helper()
//
// 	nex := func(num int) NexCLI {
// 		return NexCLI{
// 			Globals: Globals{
// 				Namespace: "system",
// 				GlobalNats: GlobalNats{
// 					NatsServers: []string{natsUrl},
// 				},
// 				// GlobalLogger: GlobalLogger{
// 				// 	Target:   []string{"std"},
// 				// 	LogLevel: "debug",
// 				// },
// 			},
// 			Node: Node{
// 				Up: Up{
// 					Agents:             []AgentConfig{},
// 					DisableNativeStart: true,
// 					NodeName:           fmt.Sprintf("testnexus-%d", num),
// 					NexusName:          "testnexus",
// 					ResourceDir:        t.TempDir(),
// 					Tags:               map[string]string{},
// 				},
// 			},
// 		}
// 	}
//
// 	for i := 0; i < size; i++ {
// 		go func() {
// 			n := nex(i)
// 			err := n.Node.Up.Run(ctx, &n.Globals)
// 			be.NilErr(t, err)
// 		}()
// 	}
// }

func TestNodeCommandDefaults(t *testing.T) {
	nex := NexCLI{
		Globals: Globals{},
	}

	parser := kong.Must(&nex,
		kong.Vars(kongVars),
		kong.Bind(&nex.Globals),
	)

	workingDir := t.TempDir()
	t.Run("UpCommand", func(t *testing.T) {
		_, err := parser.Parse([]string{"node", "up", "--resource-directory", workingDir})
		be.NilErr(t, err)

		be.Equal(t, 0, len(nex.Node.Up.Agents))
		be.False(t, nex.Node.Up.DisableNativeStart)
		be.False(t, nex.Node.Up.ShowWorkloadLogs)
		be.Equal(t, "nexus", nex.Node.Up.NexusName)
		be.Zero(t, nex.Node.Up.NodeName)
		be.Zero(t, nex.Node.Up.NodeSeed)
		be.Zero(t, nex.Node.Up.NodeXKeySeed)
		be.Equal(t, workingDir, nex.Node.Up.ResourceDir)
		be.DeepEqual(t, map[string]string(nil), nex.Node.Up.Tags)
		be.Zero(t, nex.Node.Up.State)
		be.Zero(t, nex.Node.Up.InternalNatsServerConf)
		be.Zero(t, nex.Node.Up.SigningKey)
		be.Zero(t, nex.Node.Up.RootAccountKey)
	})

	t.Run("InfoCommand", func(t *testing.T) {
		_, err := parser.Parse([]string{"node", "info", TestServerPublicKey})
		be.NilErr(t, err)

		be.True(t, nkeys.IsValidPublicServerKey(nex.Node.Info.NodeID))
	})

	t.Run("LameDuckCommand", func(t *testing.T) {
		_, err := parser.Parse([]string{"node", "lameduck", TestServerPublicKey})
		be.NilErr(t, err)

		be.True(t, nkeys.IsValidPublicServerKey(nex.Node.LameDuck.NodeID))
		be.DeepEqual(t, map[string]string(nil), nex.Node.LameDuck.Label)
		be.Equal(t, "1m0s", nex.Node.LameDuck.Delay.String())
	})

	t.Run("ListCommand", func(t *testing.T) {
		_, err := parser.Parse([]string{"node", "list"})
		be.NilErr(t, err)

		be.DeepEqual(t, map[string]string(nil), nex.Node.List.Filter)
	})
}

func TestNodeUp(t *testing.T) {
	s := startNatsServer(t)
	defer func() {
		for s.NumClients() == 0 {
			s.Shutdown()
			return
		}
	}()

	nex := NexCLI{
		Globals: Globals{
			Namespace: "system",
			GlobalNats: GlobalNats{
				NatsServers: []string{s.ClientURL()},
			},
			GlobalLogger: GlobalLogger{
				Target:   []string{"std"},
				LogLevel: "info",
			},
		},
		Node: Node{
			Up: Up{
				Agents:             []AgentConfig{},
				DisableNativeStart: true,
				NexusName:          "testnexus",
				NodeName:           "testnode",
				ResourceDir:        t.TempDir(),
				Tags:               map[string]string{},
				NodeSeed:           TestServerSeed,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	stdout := captureOutput(t, func() {
		err := nex.Node.Up.Run(ctx, &nex.Globals)
		be.NilErr(t, err)
		time.Sleep(500 * time.Millisecond)
	})

	be.True(t, strings.Contains(stdout, fmt.Sprintf("[INFO] Starting nex node version=0.0.0 commit=development build_date=unknown node_id=%s name=testnode nexus=testnexus nats_server=%s start_time=", TestServerPublicKey, s.ClientURL())))
	be.True(t, strings.Contains(stdout, "[WARN] nex node started without any agents"))
	be.True(t, strings.Contains(stdout, "[INFO] nex node ready"))
}

func TestNodeList(t *testing.T) {
	s := startNatsServer(t)
	defer func() {
		for s.NumClients() == 0 {
			s.Shutdown()
			return
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nexNodes := _test.StartNexus(t, ctx, s.ClientURL(), 3, false)

	nex := NexCLI{
		Globals: Globals{
			GlobalNats: GlobalNats{
				NatsServers: []string{s.ClientURL()},
			},
			Namespace: "system",
			JSON:      true,
		},
	}

	stdout := captureOutput(t, func() {
		err := nex.Node.List.Run(ctx, &nex.Globals)
		be.NilErr(t, err)
		time.Sleep(500 * time.Millisecond)
	})

	resp := []*models.NodePingResponse{}
	err := json.Unmarshal([]byte(stdout), &resp)
	be.NilErr(t, err)

	be.Equal(t, 3, len(resp))

	for _, node := range nexNodes {
		be.NilErr(t, node.Shutdown())
	}
}

var stdoutMu sync.Mutex

func captureOutput(t testing.TB, f func()) string {
	t.Helper()

	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	origStdout := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal("failed to create pipe: " + err.Error())
	}

	os.Stdout = w

	f()

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	if err != nil {
		t.Fatal("failed to read from pipe: " + err.Error())
	}
	r.Close()

	return buf.String()
}
