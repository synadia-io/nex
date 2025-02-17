package nex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/nats-io/nuid"
	sdk "github.com/synadia-io/nexlet.go/agent"
	tminter "github.com/synadia-labs/nex/_test/minter"
	inmem "github.com/synadia-labs/nex/_test/nexlet_inmem"
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
	defer func() {
		for s.NumClients() == 0 {
			s.Shutdown()
		}
	}()

	output := new(bytes.Buffer)
	logger := slog.New(slog.NewJSONHandler(output, nil))

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	pub, err := kp.PublicKey()
	be.NilErr(t, err)

	r, err := inmem.NewInMemAgent(pub, logger)
	be.NilErr(t, err)

	nn, err := NewNexNode(
		WithNatsConn(nc),
		WithLogger(logger),
		WithNodeKeyPair(kp),
		WithAgentRunner(r),
		WithMinter(&tminter.TestMinter{
			NatsServer: s.ClientURL(),
		}),
	)
	be.NilErr(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, nn.Shutdown())
	}()

	be.NilErr(t, nn.Start())

	for !nn.IsReady() {
		time.Sleep(100 * time.Millisecond)
	}

	be.Equal(t, 1, nn.agentCount())
	be.Equal(t, 21, nc.NumSubscriptions())
	be.Equal(t, models.NodeStateRunning, nn.state)

	jsCtx, err := jetstream.New(nc)
	be.NilErr(t, err)

	_, err = jsCtx.KeyValue(ctx, fmt.Sprintf("nex-%s", pub))
	be.NilErr(t, err)

	cancel()
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
	foundNodeStopLog := map[string]string{}

	for _, log := range logs {
		if log["msg"] == "Starting nex node" {
			foundNodeStartLog = log
			continue
		}
		if log["msg"] == "nex node stopped" {
			foundNodeStopLog = log
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, nn.Shutdown())
	}()

	be.NilErr(t, nn.Start())

	for !nn.IsReady() {
		time.Sleep(100 * time.Millisecond)
	}

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

func TestNodeLameduckHandler(t *testing.T) {
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

	for !nn.IsReady() {
		time.Sleep(100 * time.Millisecond)
	}

	req := models.LameduckRequest{
		Delay: "0s",
	}
	reqB, err := json.Marshal(req)
	be.NilErr(t, err)

	ldResp, err := nc.Request(models.LameduckRequestSubject(models.SystemNamespace, pub), reqB, time.Second*3)
	be.NilErr(t, err)

	be.NilErr(t, nn.WaitForShutdown()) // in this test, the lameduck command should shut down the node

	resp := models.LameduckResponse{}
	be.NilErr(t, json.Unmarshal(ldResp.Data, &resp))
}

func TestNodeDeployCloneUndeploy(t *testing.T) {
	s := startNatsServer(t)
	defer func() {
		for s.NumClients() == 0 {
			s.Shutdown()
		}
	}()

	output := new(bytes.Buffer)
	logger := slog.New(slog.NewJSONHandler(output, nil))

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	pub, err := kp.PublicKey()
	be.NilErr(t, err)

	r, err := inmem.NewInMemAgent(pub, logger)
	be.NilErr(t, err)

	nn, err := NewNexNode(
		WithNatsConn(nc),
		WithLogger(logger),
		WithNodeKeyPair(kp),
		WithAgentRunner(r),
		WithMinter(&tminter.TestMinter{
			NatsServer: s.ClientURL(),
		}),
	)
	be.NilErr(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	go func() {
		<-ctx.Done()
		be.NilErr(t, nn.Shutdown())
	}()

	be.NilErr(t, nn.Start())

	for !nn.IsReady() {
		time.Sleep(100 * time.Millisecond)
	}

	req := models.AuctionRequest{
		AgentType: "inmem",
		AuctionId: nuid.New().Next(),
	}
	reqB, err := json.Marshal(req)
	be.NilErr(t, err)

	auctionRespRaw, err := nc.Request(models.AuctionRequestSubject(models.SystemNamespace), reqB, time.Second)
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

	startWorkloadRespRaw, err := nc.Request(models.AuctionDeployRequestSubject(models.SystemNamespace, auctionResp.BidderId), startWorkloadReqB, time.Second)
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

	cloneWorkloadRespRaw, err := nc.Request(models.CloneWorkloadRequestSubject(models.SystemNamespace, startWorkloadResp.Id), cloneWorkloadReqB, time.Second)
	be.NilErr(t, err)

	cloneWorkloadResp := models.CloneWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(cloneWorkloadRespRaw.Data, &cloneWorkloadResp.StartWorkloadRequest))

	nsPingReq := models.AgentListWorkloadsRequest{
		Filter:    []string{},
		Namespace: models.SystemNamespace,
	}
	nsPingReqB, err := json.Marshal(nsPingReq)
	be.NilErr(t, err)

	nsPingRespRaw, err := nc.Request(models.NamespacePingRequestSubject(models.SystemNamespace), nsPingReqB, time.Second)
	be.NilErr(t, err)

	nsPingResp := models.AgentListWorkloadsResponse{}
	be.NilErr(t, json.Unmarshal(nsPingRespRaw.Data, &nsPingResp))

	stopWorkloadReq := models.StopWorkloadRequest{
		Namespace: models.SystemNamespace,
	}
	stopWorkloadReqB, err := json.Marshal(stopWorkloadReq)
	be.NilErr(t, err)

	stopWorkloadRespRaw, err := nc.Request(models.UndeployRequestSubject(models.SystemNamespace, startWorkloadResp.Id), stopWorkloadReqB, time.Second)
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
