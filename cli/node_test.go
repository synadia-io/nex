package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/carlmjohnson/be"
	"github.com/nats-io/nkeys"
)

func startNexus(t testing.TB, ctx context.Context, natsUrl string, size int) {
	t.Helper()

	nex := func(num int) NexCLI {
		return NexCLI{
			Globals: Globals{
				Namespace: "system",
				GlobalNats: GlobalNats{
					NatsServers: []string{natsUrl},
				},
				// GlobalLogger: GlobalLogger{
				// 	Target:   []string{"std"},
				// 	LogLevel: "debug",
				// },
			},
			Node: Node{
				Up: Up{
					Agents:             []AgentConfig{},
					DisableNativeStart: true,
					NodeName:           fmt.Sprintf("testnexus-%d", num),
					NexusName:          "testnexus",
					ResourceDir:        t.TempDir(),
					Tags:               map[string]string{},
					NoState:            true,
				},
			},
		}
	}

	for i := 0; i < size; i++ {
		go func() {
			n := nex(i)
			err := n.Node.Up.Run(ctx, &n.Globals)
			be.NilErr(t, err)
		}()
	}
}

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
		be.False(t, nex.Node.Up.NoState)
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
	defer s.Shutdown()

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
				NoState:            true,
				NodeSeed:           TestServerSeed,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	buffer := new(bytes.Buffer)
	reset := catchStdout(ctx, buffer)
	err := nex.Node.Up.Run(ctx, &nex.Globals)
	be.NilErr(t, err)
	time.Sleep(500 * time.Millisecond)
	reset()

	be.True(t, strings.Contains(buffer.String(), fmt.Sprintf("[INFO] Starting nex node version=0.0.0 node_id=%s name=testnode nexus=testnexus start_time=", TestServerPublicKey)))
	be.True(t, strings.Contains(buffer.String(), "[WARN] nex node started without any agents"))
	be.True(t, strings.Contains(buffer.String(), "[INFO] nex node ready"))
}

func TestNodeList(t *testing.T) {
	s := startNatsServer(t)
	defer s.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startNexus(t, ctx, s.ClientURL(), 3)
	time.Sleep(time.Second)

	nex := NexCLI{
		Globals: Globals{
			GlobalNats: GlobalNats{
				NatsServers: []string{s.ClientURL()},
			},
			Namespace: "system",
			JSON:      true,
		},
	}

	buf := new(bytes.Buffer)
	reset := catchStdout(ctx, buf)
	err := nex.Node.List.Run(ctx, &nex.Globals)
	be.NilErr(t, err)
	time.Sleep(500 * time.Millisecond)
	reset()

	// fmt.Println(buf.String())
	// resp := []*models.NodePingResponse{}
	// err = json.Unmarshal(buf.Bytes(), &resp)
	// be.NilErr(t, err)
	//
	// be.Equal(t, 3, len(resp))
}

func catchStdout(ctx context.Context, buf *bytes.Buffer) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		stdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		go func() {
			_, _ = buf.ReadFrom(r)
		}()
		<-ctx.Done()
		_ = w.Close()
		os.Stdout = stdout
		r.Close()
	}()
	return cancel
}
