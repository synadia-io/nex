package nex

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/nats-io/nuid"
	tminter "github.com/synadia-io/nex/_test/minter"
	inmem "github.com/synadia-io/nex/_test/nexlet_inmem"
	"github.com/synadia-io/nex/internal"
	"github.com/synadia-io/nex/internal/state"
	"github.com/synadia-io/nex/models"
	sdk "github.com/synadia-io/nex/sdk/go/agent"
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
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats server failed to start")
	}
	return s
}

func TestDefaultNexNodeConstructor(t *testing.T) {
	n, err := NewNexNode()
	be.NilErr(t, err)

	be.Nonzero(t, n.ctx)
	be.Nonzero(t, n.logger)
	be.Zero(t, n.startTime)
	be.False(t, n.allowRemoteAgentRegistration)
	be.Equal(t, "nexnode", n.name)
	be.Equal(t, "nexus", n.nexus)
	be.Equal(t, runtime.GOOS, n.tags[models.TagOS])
	be.Equal(t, runtime.GOARCH, n.tags[models.TagArch])
	be.Equal(t, strconv.Itoa(runtime.GOMAXPROCS(0)), n.tags[models.TagCPUs])
	be.Equal(t, "false", n.tags[models.TagLameDuck])
	be.Equal(t, models.NodeStateStarting, n.nodeState)
	be.Nonzero(t, n.nodeKeypair)
	be.Nonzero(t, n.nodeXKeypair)
	be.AllEqual(t, []*sdk.Runner{}, n.embeddedRunners)
	be.AllEqual(t, []*internal.AgentProcess{}, n.localRunners)
	be.Nonzero(t, n.agentWatcher)
	be.Nonzero(t, n.registeredAgents)
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
		{"WithInternalLogger", WithInternalNatsServer(&server.Options{Host: "1.2.3.4", Port: 10001}, &models.NatsConnectionData{}), func(t *testing.T, n *NexNode) { be.Equal(t, "nats://1.2.3.4:10001", n.server.ClientURL()) }},
		{"WithNodeKeyPair", WithNodeKeyPair(kp), func(t *testing.T, n *NexNode) { be.Equal(t, kp, n.nodeKeypair) }},
		{"WithNodeXKeyPair", WithNodeXKeyPair(xkp), func(t *testing.T, n *NexNode) { be.Equal(t, xkp, n.nodeXKeypair) }},
		{"WithState", WithState(&state.NoState{}), func(t *testing.T, n *NexNode) { be.Nonzero(t, n.state) }},
		{"WithAgentRunner", WithAgentRunner(&sdk.Runner{}), func(t *testing.T, n *NexNode) { be.Equal(t, 1, len(n.embeddedRunners)) }},
		{"WithLocalAgent", WithAgent(models.Agent{}), func(t *testing.T, n *NexNode) { be.Equal(t, 1, len(n.localRunners)) }},
		{"WithTags", WithTag("foo", "bar"), func(t *testing.T, n *NexNode) { be.Equal(t, "bar", n.tags["foo"]) }},
		{"WithAllowAgentRegistration", WithAllowRemoteAgentRegistration(), func(t *testing.T, n *NexNode) { be.True(t, n.allowRemoteAgentRegistration) }},
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

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	output := new(bytes.Buffer)

	logger := slog.New(slog.NewJSONHandler(output, nil))
	// logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
	// 	Level: slog.LevelDebug,
	// }))

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	pub, err := kp.PublicKey()
	be.NilErr(t, err)

	r, err := inmem.NewInMemAgent("nexus", pub, logger)
	be.NilErr(t, err)

	nn, err := NewNexNode(
		WithNatsConn(nc),
		WithLogger(logger),
		WithNodeKeyPair(kp),
		WithAgentRunner(r),
		WithMinter(&tminter.TestMinter{
			NatsServers: []string{s.ClientURL()},
		}),
	)
	be.NilErr(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, nn.Shutdown())
	}()

	be.NilErr(t, nn.Start())

	be.NilErr(t, nn.IsReady(10*time.Second))

	be.Equal(t, 1, nn.registeredAgents.Count())
	// Bump this whenever a node or agent endpoint is added or removed; the
	// UpdateWorkload control endpoint took it from 22 to 23, and the
	// RestartWorkload control endpoint took it from 23 to 24.
	be.Equal(t, 24, nc.NumSubscriptions())
	be.Equal(t, models.NodeStateRunning, nn.nodeState)

	cancel()
	be.NilErr(t, nn.WaitForShutdown())

	logs := []map[string]any{}
	logLines := strings.Split(output.String(), "\n")
	for i, line := range logLines {
		if i == len(logLines)-1 { // this is an empty line
			break
		}
		tLine := map[string]any{}
		err = json.Unmarshal([]byte(line), &tLine)
		be.NilErr(t, err)
		logs = append(logs, tLine)
	}

	foundNodeStartLog := map[string]string{}
	foundNodeStopLog := map[string]string{}

	for _, log := range logs {
		if logLine, ok := log["msg"].(string); ok && logLine == "Starting nex node" {
			foundNodeStartLog = map[string]string{"msg": logLine}
			continue
		}
		if logLine, ok := log["msg"].(string); ok && logLine == "nex node stopped" {
			foundNodeStopLog = map[string]string{"msg": logLine, "uptime": log["uptime"].(string)}
			continue
		}
	}

	be.Nonzero(t, foundNodeStartLog)
	be.Nonzero(t, foundNodeStopLog)

	uptime, err := time.ParseDuration(foundNodeStopLog["uptime"])
	be.NilErr(t, err)
	be.Nonzero(t, uptime)
}

func TestNodeInfoHandler(t *testing.T) {
	s := startNatsServer(t)
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	pub, err := kp.PublicKey()
	be.NilErr(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	nn, err := NewNexNode(
		WithNatsConn(nc),
		WithLogger(logger),
		WithNodeKeyPair(kp),
	)
	be.NilErr(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, nn.Shutdown())
	}()

	be.NilErr(t, nn.Start())

	be.NilErr(t, nn.IsReady(10*time.Second))

	nodeInfo, err := nc.Request(models.NodeInfoRequestSubject(models.SystemNamespace, pub), []byte{}, time.Second)
	be.NilErr(t, err)

	cancel()
	be.NilErr(t, nn.WaitForShutdown())

	resp := models.NodeInfoResponse{}
	be.NilErr(t, json.Unmarshal(nodeInfo.Data, &resp))
	be.Equal(t, pub, resp.NodeId)
	be.Equal(t, "nexnode", resp.Tags["nex.node"])
	be.Equal(t, "nexus", resp.Tags["nex.nexus"])
	be.Equal(t, runtime.GOOS, resp.Tags["nex.os"])
	be.Equal(t, runtime.GOARCH, resp.Tags["nex.arch"])
	be.Equal(t, strconv.Itoa(runtime.GOMAXPROCS(0)), resp.Tags["nex.cpucount"])
	be.Equal(t, "false", resp.Tags["nex.lameduck"])
}

func TestNodeLameduckHandlerWithTag(t *testing.T) {
	s := startNatsServer(t)
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	pub, err := kp.PublicKey()
	be.NilErr(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	nn, err := NewNexNode(
		WithNatsConn(nc),
		WithLogger(logger),
		WithNodeKeyPair(kp),
		WithTag("foo", "bar"),
	)
	be.NilErr(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, nn.Shutdown())
	}()

	be.NilErr(t, nn.Start())

	be.NilErr(t, nn.IsReady(10*time.Second))

	req := models.LameduckRequest{
		Delay: "0s",
		Tag:   map[string]string{"foo": "bar"},
	}
	reqB, err := json.Marshal(req)
	be.NilErr(t, err)

	ldResp, err := nc.Request(models.LameduckRequestSubject(models.SystemNamespace, pub), reqB, time.Second*3)
	be.NilErr(t, err)

	resp := models.LameduckResponse{}
	be.NilErr(t, json.Unmarshal(ldResp.Data, &resp))
	be.True(t, resp.Success)

	be.Equal(t, models.ErrLameduckShutdown, nn.WaitForShutdown())
	be.NilErr(t, nn.Shutdown())
}

func TestNodeLameduckHandlerWithoutTag(t *testing.T) {
	s := startNatsServer(t)
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	pub, err := kp.PublicKey()
	be.NilErr(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	nn, err := NewNexNode(
		WithNatsConn(nc),
		WithLogger(logger),
		WithNodeKeyPair(kp),
	)
	be.NilErr(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, nn.Shutdown())
	}()

	be.NilErr(t, nn.Start())

	be.NilErr(t, nn.IsReady(10*time.Second))

	req := models.LameduckRequest{
		Delay: "0s",
		Tag:   map[string]string{"foo": "bar"},
	}
	reqB, err := json.Marshal(req)
	be.NilErr(t, err)

	ldResp, err := nc.Request(models.LameduckRequestSubject(models.SystemNamespace, pub), reqB, time.Second*3)
	be.Equal(t, nats.ErrTimeout, err)
	be.Zero(t, ldResp)

	be.NilErr(t, nn.Shutdown())
}

func TestNodeShutdownExitCodes(t *testing.T) {
	t.Run("normal shutdown returns nil", func(t *testing.T) {
		s := startNatsServer(t)
		defer s.Shutdown()

		nc, err := nats.Connect(s.ClientURL())
		be.NilErr(t, err)
		defer nc.Close()

		kp, err := nkeys.CreateServer()
		be.NilErr(t, err)

		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		nn, err := NewNexNode(
			WithNatsConn(nc),
			WithLogger(logger),
			WithNodeKeyPair(kp),
		)
		be.NilErr(t, err)

		be.NilErr(t, nn.Start())

		be.NilErr(t, nn.IsReady(10*time.Second))

		// Normal shutdown
		go func() {
			time.Sleep(100 * time.Millisecond)
			be.NilErr(t, nn.Shutdown())
		}()

		err = nn.WaitForShutdown()
		be.NilErr(t, err) // Normal shutdown should return nil
	})

	t.Run("lameduck shutdown returns ErrLameduckShutdown", func(t *testing.T) {
		s := startNatsServer(t)
		defer s.Shutdown()

		nc, err := nats.Connect(s.ClientURL())
		be.NilErr(t, err)
		defer nc.Close()

		kp, err := nkeys.CreateServer()
		be.NilErr(t, err)

		pub, err := kp.PublicKey()
		be.NilErr(t, err)

		logger := slog.New(slog.NewTextHandler(io.Discard, nil))

		nn, err := NewNexNode(
			WithNatsConn(nc),
			WithLogger(logger),
			WithNodeKeyPair(kp),
		)
		be.NilErr(t, err)

		be.NilErr(t, nn.Start())

		be.NilErr(t, nn.IsReady(10*time.Second))

		// Trigger lameduck
		req := models.LameduckRequest{
			Delay: "100ms",
		}
		reqB, err := json.Marshal(req)
		be.NilErr(t, err)

		_, err = nc.Request(models.LameduckRequestSubject(models.SystemNamespace, pub), reqB, time.Second)
		be.NilErr(t, err)

		err = nn.WaitForShutdown()
		be.Equal(t, models.ErrLameduckShutdown, err) // Lameduck shutdown should return specific error
	})
}

// failureLogBuffer collects a node's slog output and replays it when the test
// fails; otherwise the logs vanish with the buffer and a CI-only failure (the
// auction timeout seen on ubuntu runners) leaves no evidence. The mutex is
// required: node goroutines still log while t.Cleanup reads.
type failureLogBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (f *failureLogBuffer) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.b.Write(p)
}

func (f *failureLogBuffer) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.b.String()
}

func dumpLogsOnFailure(t *testing.T, output *failureLogBuffer) {
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("node logs:\n%s", output.String())
		}
	})
}

func TestNodeDeployCloneUndeploy(t *testing.T) {
	s := startNatsServer(t)
	defer s.Shutdown()

	output := new(failureLogBuffer)
	dumpLogsOnFailure(t, output)
	logger := slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: slog.LevelDebug}))

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	pub, err := kp.PublicKey()
	be.NilErr(t, err)

	r, err := inmem.NewInMemAgent("nexus", pub, logger)
	be.NilErr(t, err)

	nn, err := NewNexNode(
		WithNatsConn(nc),
		WithLogger(logger),
		WithNodeKeyPair(kp),
		WithAgentRunner(r),
		WithMinter(&tminter.TestMinter{
			NatsServers: []string{s.ClientURL()},
		}),
	)
	be.NilErr(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, nn.Shutdown())
	}()

	be.NilErr(t, nn.Start())

	be.NilErr(t, nn.IsReady(10*time.Second))

	req := models.AuctionRequest{
		AgentType: "inmem",
		AuctionId: nuid.New().Next(),
	}
	reqB, err := json.Marshal(req)
	be.NilErr(t, err)

	auctionRespRaw, err := nc.Request(models.AuctionRequestSubject(models.SystemNamespace), reqB, time.Second*10)
	be.NilErr(t, err)

	auctionResp := models.AuctionResponse{}
	be.NilErr(t, json.Unmarshal(auctionRespRaw.Data, &auctionResp))

	startWorkloadReq := models.StartWorkloadRequest{
		Description:       "test",
		Name:              "test",
		Namespace:         models.SystemNamespace,
		RunRequest:        "{}",
		WorkloadLifecycle: "service",
		WorkloadType:      "inmem",
	}
	startWorkloadReqB, err := json.Marshal(startWorkloadReq)
	be.NilErr(t, err)

	startWorkloadRespRaw, err := nc.Request(models.AuctionDeployRequestSubject(models.SystemNamespace, auctionResp.BidderId), startWorkloadReqB, time.Second*5)
	be.NilErr(t, err)

	startWorkloadResp := models.StartWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(startWorkloadRespRaw.Data, &startWorkloadResp))

	xkp, err := nkeys.CreateCurveKeys()
	be.NilErr(t, err)
	xkpub, err := xkp.PublicKey()
	be.NilErr(t, err)

	cloneWorkloadReq := models.CloneWorkloadRequest{
		Namespace:     models.SystemNamespace,
		NewTargetXkey: xkpub,
	}
	cloneWorkloadReqB, err := json.Marshal(cloneWorkloadReq)
	be.NilErr(t, err)

	cloneWorkloadRespRaw, err := nc.Request(models.CloneWorkloadRequestSubject(models.SystemNamespace, startWorkloadResp.Id), cloneWorkloadReqB, time.Second*5)
	be.NilErr(t, err)

	cloneWorkloadResp := models.CloneWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(cloneWorkloadRespRaw.Data, &cloneWorkloadResp.StartWorkloadRequest))

	nsPingReq := models.AgentListWorkloadsRequest{
		Filter:    []string{},
		Namespace: models.SystemNamespace,
	}
	nsPingReqB, err := json.Marshal(nsPingReq)
	be.NilErr(t, err)

	nsPingRespRaw, err := nc.Request(models.NamespacePingRequestSubject(models.SystemNamespace), nsPingReqB, time.Second*10)
	be.NilErr(t, err)

	nsPingResp := models.AgentListWorkloadsResponse{}
	be.NilErr(t, json.Unmarshal(nsPingRespRaw.Data, &nsPingResp))

	stopWorkloadReq := models.StopWorkloadRequest{
		Namespace: models.SystemNamespace,
	}
	stopWorkloadReqB, err := json.Marshal(stopWorkloadReq)
	be.NilErr(t, err)

	stopWorkloadRespRaw, err := nc.Request(models.UndeployRequestSubject(models.SystemNamespace, startWorkloadResp.Id), stopWorkloadReqB, time.Second*5)
	be.NilErr(t, err)

	stopWorkloadResp := models.StopWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(stopWorkloadRespRaw.Data, &stopWorkloadResp))

	cancel()
	be.NilErr(t, nn.WaitForShutdown())

	// Test Auction Response
	be.Nonzero(t, auctionResp.BidderId)
	be.Equal(t, `{}`, auctionResp.StartRequestSchema)
	be.AllEqual(t, []models.WorkloadLifecycle{models.WorkloadLifecycleService, models.WorkloadLifecycleJob, models.WorkloadLifecycleFunction}, auctionResp.SupportedLifecycles)
	be.Nonzero(t, auctionResp.Xkey)

	// Test Start Workload Response
	be.Nonzero(t, startWorkloadResp.Id)
	be.Nonzero(t, startWorkloadResp.Name)

	// Test Clone Workload Response
	be.Nonzero(t, cloneWorkloadResp.StartWorkloadRequest)
	be.Equal(t, startWorkloadReq.Name, cloneWorkloadResp.StartWorkloadRequest.Name)
	be.Equal(t, startWorkloadReq.Description, cloneWorkloadResp.StartWorkloadRequest.Description)
	be.Equal(t, startWorkloadReq.Namespace, cloneWorkloadResp.StartWorkloadRequest.Namespace)
	be.Equal(t, startWorkloadReq.RunRequest, cloneWorkloadResp.StartWorkloadRequest.RunRequest)
	be.Equal(t, startWorkloadReq.WorkloadLifecycle, cloneWorkloadResp.StartWorkloadRequest.WorkloadLifecycle)
	be.Equal(t, startWorkloadReq.WorkloadType, cloneWorkloadResp.StartWorkloadRequest.WorkloadType)

	// Test Namespace Ping Response
	be.Equal(t, 1, len(nsPingResp))
	be.Equal(t, startWorkloadResp.Id, nsPingResp[0].Id)
	be.Equal(t, startWorkloadResp.Name, nsPingResp[0].Name)

	// Test Stop Workload Response
	be.Equal(t, startWorkloadResp.Id, stopWorkloadResp.Id)
	be.True(t, stopWorkloadResp.Stopped)
}

// TestNodeControlServiceRebuildsAfterUnexpectedStop proves the node's control
// micro service recovers on its own after an unexpected stop (which nats.go
// micro does itself on a connection close or an async endpoint error), and
// that a graceful Shutdown does NOT rebuild it.
func TestNodeControlServiceRebuildsAfterUnexpectedStop(t *testing.T) {
	s := startNatsServer(t)
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)
	pub, err := kp.PublicKey()
	be.NilErr(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nn, err := NewNexNode(WithNatsConn(nc), WithLogger(logger), WithNodeKeyPair(kp))
	be.NilErr(t, err)
	be.NilErr(t, nn.Start())
	be.NilErr(t, nn.IsReady(10*time.Second))

	infoSubj := models.NodeInfoRequestSubject(models.SystemNamespace, pub)

	// Endpoint serves before the stop.
	_, err = nc.Request(infoSubj, nil, time.Second)
	be.NilErr(t, err)

	// Simulate micro auto-stopping the service. Its DoneHandler must rebuild.
	nn.serviceMu.Lock()
	old := nn.service
	nn.serviceMu.Unlock()
	be.NilErr(t, old.Stop())
	be.True(t, old.Stopped())

	// Endpoint serves again once the rebuild lands.
	served := false
	for i := 0; i < 100; i++ {
		if _, rerr := nc.Request(infoSubj, nil, 200*time.Millisecond); rerr == nil {
			served = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	be.True(t, served)

	// A fresh, non-stopped service replaced the old one.
	nn.serviceMu.Lock()
	rebuilt := nn.service
	nn.serviceMu.Unlock()
	be.True(t, rebuilt != old)
	be.True(t, !rebuilt.Stopped())

	// Graceful shutdown gates the rebuild off and leaves the service stopped.
	be.NilErr(t, nn.Shutdown())
	be.True(t, nn.shuttingDown.Load())
	be.True(t, rebuilt.Stopped())
}

// TestNodeBuildServiceRefusesCommitDuringShutdown proves the rebuild path
// cannot commit (or leak) a new control service once a graceful shutdown has
// begun -- closing the shutdown-vs-rebuild race.
func TestNodeBuildServiceRefusesCommitDuringShutdown(t *testing.T) {
	s := startNatsServer(t)
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nn, err := NewNexNode(WithNatsConn(nc), WithLogger(logger), WithNodeKeyPair(kp))
	be.NilErr(t, err)
	be.NilErr(t, nn.Start())
	be.NilErr(t, nn.IsReady(10*time.Second))

	nn.serviceMu.Lock()
	committed := nn.service
	nn.serviceMu.Unlock()

	// Shutdown has begun; a concurrent rebuild attempt must not commit.
	nn.shuttingDown.Store(true)
	be.NilErr(t, nn.buildMicroService())

	nn.serviceMu.Lock()
	after := nn.service
	nn.serviceMu.Unlock()
	be.True(t, after == committed) // unchanged; the freshly-built service was stopped, not committed

	nn.shuttingDown.Store(false)
	be.NilErr(t, nn.Shutdown())
}
