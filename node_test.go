package nex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nkeys"
	sdk "github.com/synadia-io/nexlet.go/agent"
	"github.com/synadia-labs/nex/internal"
	"github.com/synadia-labs/nex/models"
)

func startNatsServer(t testing.TB) *server.Server {
	t.Helper()

	s, err := server.NewServer(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	})
	be.NilErr(t, err)

	s.Start()
	return s
}

func TestDefaultNexNodeConstructor(t *testing.T) {
	n, err := NewNexNode()
	be.NilErr(t, err)

	be.Nonzero(t, n.ctx)
	be.Nonzero(t, n.logger)
	be.Zero(t, n.startTime)
	be.False(t, n.allowAgentRegistration)
	be.False(t, n.noState)
	be.Equal(t, "nexnode", n.name)
	be.Equal(t, "nexus", n.nexus)
	be.Equal(t, runtime.GOOS, n.tags[models.TagOS])
	be.Equal(t, runtime.GOARCH, n.tags[models.TagArch])
	be.Equal(t, strconv.Itoa(runtime.GOMAXPROCS(0)), n.tags[models.TagCPUs])
	be.Equal(t, "false", n.tags[models.TagLameDuck])
	be.Equal(t, models.NodeStateStarting, n.state)
	be.Nonzero(t, n.nodeKeypair)
	be.Nonzero(t, n.nodeXKeypair)
	be.AllEqual(t, []*sdk.Runner{}, n.embeddedRunners)
	be.AllEqual(t, []*internal.AgentProcess{}, n.localRunners)
	be.Nonzero(t, n.agentWatcher)
	be.Nonzero(t, n.regs)
	be.Zero(t, n.nc)
	be.Zero(t, n.server)
	be.Nonzero(t, n.auctionMap)
	be.Nonzero(t, n.nodeShutdown)
}

func TestNodeOptions(t *testing.T) {
	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)
	xkp, err := nkeys.CreateCurveKeys()
	be.NilErr(t, err)
	tt := []struct {
		name      string
		opt       NexNodeOption
		assertion func(*testing.T, *NexNode)
	}{
		{"WithContext", WithContext(context.WithValue(context.Background(), "foo", "bar")), func(t *testing.T, n *NexNode) { be.Equal(t, "bar", n.ctx.Value("foo")) }}, //nolint
		{"WithInternalLogger", WithInternalNatsServer(&server.Options{Host: "1.2.3.4", Port: 10001}), func(t *testing.T, n *NexNode) { be.Equal(t, "nats://1.2.3.4:10001", n.server.ClientURL()) }},
		{"WithNodeKeyPair", WithNodeKeyPair(kp), func(t *testing.T, n *NexNode) { be.Equal(t, kp, n.nodeKeypair) }},
		{"WithNodeXKeyPair", WithNodeXKeyPair(xkp), func(t *testing.T, n *NexNode) { be.Equal(t, xkp, n.nodeXKeypair) }},
		{"WithNoState", WithNoState(), func(t *testing.T, n *NexNode) { be.True(t, n.noState) }},
		{"WithAgentRunner", WithAgentRunner(&sdk.Runner{}), func(t *testing.T, n *NexNode) { be.Equal(t, 1, len(n.embeddedRunners)) }},
		{"WithLocalAgent", WithLocalAgent(models.Agent{}), func(t *testing.T, n *NexNode) { be.Equal(t, 1, len(n.localRunners)) }},
		{"WithTags", WithTag("foo", "bar"), func(t *testing.T, n *NexNode) { be.Equal(t, "bar", n.tags["foo"]) }},
		{"WithAllowAgentRegistration", WithAllowAgentRegistration(), func(t *testing.T, n *NexNode) { be.True(t, n.allowAgentRegistration) }},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			n, err := NewNexNode(tc.opt)
			be.NilErr(t, err)
			tc.assertion(t, n)
		})
	}
}

func TestNodeStartStop(t *testing.T) {
	s := startNatsServer(t)
	defer s.Shutdown()

	output := new(bytes.Buffer)
	logger := slog.New(slog.NewJSONHandler(output, nil))

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	pub, err := kp.PublicKey()
	be.NilErr(t, err)

	nn, err := NewNexNode(
		WithNatsConn(nc),
		WithLogger(logger),
		WithNodeKeyPair(kp),
	)
	be.NilErr(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, nn.Shutdown())
	}()

	be.NilErr(t, nn.Start())

	be.Equal(t, 21, nc.NumSubscriptions())
	be.Equal(t, models.NodeStateRunning, nn.state)

	jsCtx, err := jetstream.New(nc)
	be.NilErr(t, err)

	_, err = jsCtx.KeyValue(ctx, fmt.Sprintf("nex-%s", pub))
	be.NilErr(t, err)

	be.Equal(t, 0, nn.agentCount())

	be.NilErr(t, nn.WaitForShutdown())

	logs := []map[string]string{}

	logLines := strings.Split(output.String(), "\n")
	for i, line := range logLines {
		if i == len(logLines)-1 { // this is an empty line
			break
		}
		tLine := map[string]string{}
		err = json.Unmarshal([]byte(line), &tLine)
		be.NilErr(t, err)
		logs = append(logs, tLine)
	}

	foundNodeStartLog := map[string]string{}
	foundNodeNoAgents := map[string]string{}
	foundNodeStopLog := map[string]string{}

	for _, log := range logs {
		if log["msg"] == "Starting nex node" {
			foundNodeStartLog = log
			continue
		}
		if log["msg"] == "nex node started without any agents" {
			foundNodeNoAgents = log
		}
		if log["msg"] == "nex node stopped" {
			foundNodeStopLog = log
			continue
		}
	}

	be.Nonzero(t, foundNodeStartLog)
	be.Nonzero(t, foundNodeNoAgents)
	be.Nonzero(t, foundNodeStopLog)

	uptime, err := time.ParseDuration(foundNodeStopLog["uptime"])
	be.NilErr(t, err)

	be.True(t, uptime.Seconds() > 3.0 && uptime.Seconds() < 3.1) // controlled by the context timeout above
}
