package nodecontrol

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"disorder.dev/shandler"
	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"

	"github.com/synadia-io/nex/api/nodecontrol/gen"
	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/nex/node"
)

const (
	Node1ServerSeed      string = "SNAB2T3VG2363NDA2JK7NT5O3FN5VCXI2MYJHOPFO2NIDXQU6DIWQTBQC4"
	Node1ServerPublicKey string = "NCUU2YIYXEPGTCDXDKQR7LL5PXDHIDG7SDFLWKE3WY63ZGCZL2HKIAJT"
	Node1XKeySeed        string = "SXAOUP7RZFW5QPE2GDWTPABUDM5UIAK6BPULJPWZQAFFL2RZ5K3UYWHYY4"
	Node1XkeyPublicKey   string = "XAL54S5FE6SRPONXRNVE4ZDAOHOT44GFIY2ZW33DHLR2U3H2HJSXXRKY"
)

func buildTestBinary(t testing.TB, binMain string, workingDir string) string {
	t.Helper()
	binPath := func() string {
		binName := "test"
		if runtime.GOOS == "windows" {
			binName = "test.exe"
		}
		return filepath.Join(workingDir, binName)
	}()

	if _, err := os.Stat(binPath); err == nil {
		return binPath
	}

	cmd := exec.Command("go", "build", "-o", binPath, binMain)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	be.NilErr(t, cmd.Run())

	_, err := os.Stat(binPath)
	be.NilErr(t, err)

	return binPath
}

func startNatsServer(t testing.TB, workingDir string) *server.Server {
	t.Helper()

	s := server.New(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  workingDir,
	})

	s.Start()

	go s.WaitForShutdown()

	return s
}

func startNexus(t testing.TB, ctx context.Context, logger *slog.Logger, workingDir, natsUrl string, numNodes int) {
	t.Helper()

	nc, err := nats.Connect(natsUrl)
	be.NilErr(t, err)

	for i := 0; i < numNodes; i++ {
		var kp, xkp nkeys.KeyPair
		if i == 0 {
			kp, err = nkeys.FromSeed([]byte(Node1ServerSeed))
			be.NilErr(t, err)
			xkp, err = nkeys.FromSeed([]byte(Node1XKeySeed))
			be.NilErr(t, err)
		} else {
			kp, err = nkeys.CreateServer()
			be.NilErr(t, err)
			xkp, err = nkeys.CreateCurveKeys()
			be.NilErr(t, err)
		}
		nn, err := node.NewNexNode(kp, nc,
			models.WithContext(ctx),
			models.WithLogger(logger),
			models.WithXKeyKeyPair(xkp),
			models.WithNodeName(fmt.Sprintf("node-%d", i+1)),
			models.WithNexus("testnexus"),
			models.WithResourceDirectory(workingDir),
		)
		be.NilErr(t, err)

		err = nn.Validate()
		be.NilErr(t, err)

		go func() {
			err = nn.Start()
			be.NilErr(t, err)
		}()
	}
}

func TestAuction(t *testing.T) {
	workingDir := t.TempDir()
	natsServer := startNatsServer(t, workingDir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)

	t.Cleanup(func() {
		os.RemoveAll(filepath.Join(os.TempDir(), "inex-"+Node1ServerPublicKey))
		cancel()
		natsServer.Shutdown()
	})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	logger := slog.New(shandler.NewHandler(
		shandler.WithLogLevel(slog.LevelDebug),
		shandler.WithGroupFilter([]string{"actor_system"}),
		shandler.WithStdOut(stdout),
		shandler.WithStdErr(stderr),
	))

	startNexus(t, ctx, logger, workingDir, natsServer.ClientURL(), 1)
	time.Sleep(1000 * time.Millisecond)

	nc, err := nats.Connect(natsServer.ClientURL())
	be.NilErr(t, err)

	control, err := NewControlApiClient(nc, logger)
	be.NilErr(t, err)

	resp, err := control.Auction(models.NodeSystemNamespace, map[string]string{})
	be.NilErr(t, err)

	be.Equal(t, 1, len(resp))

	resp, err = control.Auction(models.NodeSystemNamespace, map[string]string{models.TagNodeName: "notreal"})
	be.NilErr(t, err)

	be.Equal(t, 0, len(resp))
}

func TestPing(t *testing.T) {
	workingDir := t.TempDir()
	natsServer := startNatsServer(t, workingDir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)

	t.Cleanup(func() {
		os.RemoveAll(filepath.Join(os.TempDir(), "inex-"+Node1ServerPublicKey))
		cancel()
		natsServer.Shutdown()
	})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	logger := slog.New(shandler.NewHandler(
		shandler.WithLogLevel(slog.LevelDebug),
		shandler.WithGroupFilter([]string{"actor_system"}),
		shandler.WithStdOut(stdout),
		shandler.WithStdErr(stderr),
	))

	startNexus(t, ctx, logger, workingDir, natsServer.ClientURL(), 5)
	time.Sleep(1000 * time.Millisecond)

	nc, err := nats.Connect(natsServer.ClientURL())
	be.NilErr(t, err)

	control, err := NewControlApiClient(nc, logger)
	be.NilErr(t, err)

	resp, err := control.Ping()
	be.NilErr(t, err)

	be.Equal(t, 5, len(resp))
}

func TestDirectPing(t *testing.T) {
	workingDir := t.TempDir()
	natsServer := startNatsServer(t, workingDir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)

	t.Cleanup(func() {
		os.RemoveAll(filepath.Join(os.TempDir(), "inex-"+Node1ServerPublicKey))
		cancel()
		natsServer.Shutdown()
	})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	logger := slog.New(shandler.NewHandler(
		shandler.WithLogLevel(slog.LevelDebug),
		shandler.WithGroupFilter([]string{"actor_system"}),
		shandler.WithStdOut(stdout),
		shandler.WithStdErr(stderr),
	))

	startNexus(t, ctx, logger, workingDir, natsServer.ClientURL(), 1)
	time.Sleep(1000 * time.Millisecond)

	nc, err := nats.Connect(natsServer.ClientURL())
	be.NilErr(t, err)

	control, err := NewControlApiClient(nc, logger)
	be.NilErr(t, err)

	resp, err := control.DirectPing(Node1ServerPublicKey)
	be.NilErr(t, err)

	be.Equal(t, Node1ServerPublicKey, resp.NodeId)
}

func TestAuctionDeployAndFindWorkload(t *testing.T) {
	workingDir := t.TempDir()
	natsServer := startNatsServer(t, workingDir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)

	t.Cleanup(func() {
		os.RemoveAll(filepath.Join(os.TempDir(), "inex-"+Node1ServerPublicKey))
		cancel()
		natsServer.Shutdown()
	})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	logger := slog.New(shandler.NewHandler(
		shandler.WithLogLevel(slog.LevelDebug),
		shandler.WithGroupFilter([]string{"actor_system"}),
		shandler.WithStdOut(stdout),
		shandler.WithStdErr(stderr),
	))

	startNexus(t, ctx, logger, workingDir, natsServer.ClientURL(), 1)
	time.Sleep(1000 * time.Millisecond)

	nc, err := nats.Connect(natsServer.ClientURL())
	be.NilErr(t, err)

	control, err := NewControlApiClient(nc, logger)
	be.NilErr(t, err)

	auctionResp, err := control.Auction(models.NodeSystemNamespace, map[string]string{})
	be.NilErr(t, err)

	env := make(map[string]string)
	envB, err := json.Marshal(env)
	be.NilErr(t, err)

	tAKey, err := nkeys.CreateCurveKeys()
	be.NilErr(t, err)

	tAPub, err := tAKey.PublicKey()
	be.NilErr(t, err)

	encEnv, err := tAKey.Seal(envB, auctionResp[0].TargetXkey)
	be.NilErr(t, err)

	binPath := buildTestBinary(t, "../../test/testdata/forever/main.go", workingDir)

	resp, err := control.AuctionDeployWorkload(models.NodeSystemNamespace, auctionResp[0].BidderId, gen.StartWorkloadRequestJson{
		Description:     "Test Workload",
		Namespace:       models.NodeSystemNamespace,
		RetryCount:      3,
		Uri:             "file://" + binPath,
		WorkloadName:    "testworkload",
		WorkloadRuntype: "service",
		WorkloadType:    "direct-start",
		EncEnvironment: gen.SharedEncEnvJson{
			Base64EncryptedEnv: base64.StdEncoding.EncodeToString(encEnv),
			EncryptedBy:        tAPub,
		},
	})
	be.NilErr(t, err)

	be.True(t, resp.Started)

	pingResp, err := control.FindWorkload(models.NodeSystemNamespace, resp.Id)
	be.NilErr(t, err)

	be.Equal(t, resp.Id, pingResp.WorkloadSummary.Id)
	be.Equal(t, models.WorkloadStateRunning, pingResp.WorkloadSummary.WorkloadState)

	_, err = control.FindWorkload("badnamespace", resp.Id)
	be.Equal(t, nats.ErrTimeout, err)
}

func TestDirectDeployAndListWorkloads(t *testing.T) {
	workingDir := t.TempDir()
	natsServer := startNatsServer(t, workingDir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)

	t.Cleanup(func() {
		os.RemoveAll(filepath.Join(os.TempDir(), "inex-"+Node1ServerPublicKey))
		cancel()
		natsServer.Shutdown()
	})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	logger := slog.New(shandler.NewHandler(
		shandler.WithLogLevel(slog.LevelDebug),
		shandler.WithGroupFilter([]string{"actor_system"}),
		shandler.WithStdOut(stdout),
		shandler.WithStdErr(stderr),
	))

	startNexus(t, ctx, logger, workingDir, natsServer.ClientURL(), 1)
	time.Sleep(1000 * time.Millisecond)

	nc, err := nats.Connect(natsServer.ClientURL())
	be.NilErr(t, err)

	control, err := NewControlApiClient(nc, logger)
	be.NilErr(t, err)

	env := make(map[string]string)
	envB, err := json.Marshal(env)
	be.NilErr(t, err)

	tAKey, err := nkeys.CreateCurveKeys()
	be.NilErr(t, err)

	tAPub, err := tAKey.PublicKey()
	be.NilErr(t, err)

	encEnv, err := tAKey.Seal(envB, Node1XkeyPublicKey)
	be.NilErr(t, err)

	binPath := buildTestBinary(t, "../../test/testdata/forever/main.go", workingDir)

	resp, err := control.DeployWorkload(models.NodeSystemNamespace, Node1ServerPublicKey, gen.StartWorkloadRequestJson{
		Description:     "Test Workload",
		Namespace:       models.NodeSystemNamespace,
		RetryCount:      3,
		Uri:             "file://" + binPath,
		WorkloadName:    "testworkload",
		WorkloadRuntype: "service",
		WorkloadType:    "direct-start",
		EncEnvironment: gen.SharedEncEnvJson{
			Base64EncryptedEnv: base64.StdEncoding.EncodeToString(encEnv),
			EncryptedBy:        tAPub,
		},
	})
	be.NilErr(t, err)

	be.True(t, resp.Started)

	wl, err := control.ListWorkloads(models.NodeSystemNamespace)
	be.NilErr(t, err)

	be.Equal(t, 1, len(wl))

	wl, err = control.ListWorkloads("badnamespace")
	be.NilErr(t, err)

	be.Equal(t, 0, len(wl))
}

func TestUndeployWorkload(t *testing.T) {
	workingDir := t.TempDir()
	natsServer := startNatsServer(t, workingDir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)

	t.Cleanup(func() {
		os.RemoveAll(filepath.Join(os.TempDir(), "inex-"+Node1ServerPublicKey))
		cancel()
		natsServer.Shutdown()
	})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	logger := slog.New(shandler.NewHandler(
		shandler.WithLogLevel(slog.LevelDebug),
		shandler.WithGroupFilter([]string{"actor_system"}),
		shandler.WithStdOut(stdout),
		shandler.WithStdErr(stderr),
	))

	startNexus(t, ctx, logger, workingDir, natsServer.ClientURL(), 1)
	time.Sleep(1000 * time.Millisecond)

	nc, err := nats.Connect(natsServer.ClientURL())
	be.NilErr(t, err)

	control, err := NewControlApiClient(nc, logger)
	be.NilErr(t, err)

	env := make(map[string]string)
	envB, err := json.Marshal(env)
	be.NilErr(t, err)

	tAKey, err := nkeys.CreateCurveKeys()
	be.NilErr(t, err)

	tAPub, err := tAKey.PublicKey()
	be.NilErr(t, err)

	encEnv, err := tAKey.Seal(envB, Node1XkeyPublicKey)
	be.NilErr(t, err)

	binPath := buildTestBinary(t, "../../test/testdata/forever/main.go", workingDir)

	resp, err := control.DeployWorkload(models.NodeSystemNamespace, Node1ServerPublicKey, gen.StartWorkloadRequestJson{
		Description:     "Test Workload",
		Namespace:       models.NodeSystemNamespace,
		RetryCount:      3,
		Uri:             "file://" + binPath,
		WorkloadName:    "testworkload",
		WorkloadRuntype: "service",
		WorkloadType:    "direct-start",
		EncEnvironment: gen.SharedEncEnvJson{
			Base64EncryptedEnv: base64.StdEncoding.EncodeToString(encEnv),
			EncryptedBy:        tAPub,
		},
	})
	be.NilErr(t, err)

	be.True(t, resp.Started)

	stopResp, err := control.UndeployWorkload(models.NodeSystemNamespace, resp.Id)
	be.NilErr(t, err)

	be.True(t, stopResp.Stopped)

	wl, err := control.ListWorkloads(models.NodeSystemNamespace)
	be.NilErr(t, err)

	be.Equal(t, 0, len(wl))
}

func TestGetNodeInfo(t *testing.T) {
	workingDir := t.TempDir()
	natsServer := startNatsServer(t, workingDir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)

	t.Cleanup(func() {
		os.RemoveAll(filepath.Join(os.TempDir(), "inex-"+Node1ServerPublicKey))
		cancel()
		natsServer.Shutdown()
	})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	logger := slog.New(shandler.NewHandler(
		shandler.WithLogLevel(slog.LevelDebug),
		shandler.WithGroupFilter([]string{"actor_system"}),
		shandler.WithStdOut(stdout),
		shandler.WithStdErr(stderr),
	))

	startNexus(t, ctx, logger, workingDir, natsServer.ClientURL(), 1)
	time.Sleep(1000 * time.Millisecond)

	nc, err := nats.Connect(natsServer.ClientURL())
	be.NilErr(t, err)

	control, err := NewControlApiClient(nc, logger)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := control.GetInfo(Node1ServerPublicKey, gen.NodeInfoRequestJson{
		Namespace: models.NodeSystemNamespace,
	})
	be.NilErr(t, err)

	be.Equal(t, Node1ServerPublicKey, resp.NodeId)
	be.Equal(t, "node-1", resp.Tags.Tags[models.TagNodeName])
	be.Equal(t, "testnexus", resp.Tags.Tags[models.TagNexus])
	be.Equal(t, 0, len(resp.WorkloadSummaries))
}

func TestSetLameduck(t *testing.T) {
	workingDir := t.TempDir()
	natsServer := startNatsServer(t, workingDir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)

	t.Cleanup(func() {
		os.RemoveAll(filepath.Join(os.TempDir(), "inex-"+Node1ServerPublicKey))
		cancel()
		natsServer.Shutdown()
	})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	logger := slog.New(shandler.NewHandler(
		shandler.WithLogLevel(slog.LevelDebug),
		shandler.WithGroupFilter([]string{"actor_system"}),
		shandler.WithStdOut(stdout),
		shandler.WithStdErr(stderr),
	))

	startNexus(t, ctx, logger, workingDir, natsServer.ClientURL(), 1)
	time.Sleep(1000 * time.Millisecond)

	nc, err := nats.Connect(natsServer.ClientURL())
	be.NilErr(t, err)

	control, err := NewControlApiClient(nc, logger)
	be.NilErr(t, err)

	_, err = control.SetLameDuck(Node1ServerPublicKey, time.Second*3)
	be.NilErr(t, err)

	resp, err := control.GetInfo(Node1ServerPublicKey, gen.NodeInfoRequestJson{
		Namespace: models.NodeSystemNamespace,
	})
	be.NilErr(t, err)

	ld, err := strconv.ParseBool(resp.Tags.Tags[models.TagLameDuck])
	be.NilErr(t, err)
	be.True(t, ld)
}

func TestCopyWorkload(t *testing.T) {
	workingDir := t.TempDir()
	natsServer := startNatsServer(t, workingDir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)

	t.Cleanup(func() {
		os.RemoveAll(filepath.Join(os.TempDir(), "inex-"+Node1ServerPublicKey))
		cancel()
		natsServer.Shutdown()
	})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	logger := slog.New(shandler.NewHandler(
		shandler.WithLogLevel(slog.LevelDebug),
		shandler.WithGroupFilter([]string{"actor_system"}),
		shandler.WithStdOut(stdout),
		shandler.WithStdErr(stderr),
	))

	startNexus(t, ctx, logger, workingDir, natsServer.ClientURL(), 5)
	time.Sleep(1000 * time.Millisecond)

	nc, err := nats.Connect(natsServer.ClientURL())
	be.NilErr(t, err)

	control, err := NewControlApiClient(nc, logger)
	be.NilErr(t, err)

	env := make(map[string]string)
	envB, err := json.Marshal(env)
	be.NilErr(t, err)

	tAKey, err := nkeys.CreateCurveKeys()
	be.NilErr(t, err)

	tAPub, err := tAKey.PublicKey()
	be.NilErr(t, err)

	encEnv, err := tAKey.Seal(envB, Node1XkeyPublicKey)
	be.NilErr(t, err)

	binPath := buildTestBinary(t, "../../test/testdata/forever/main.go", workingDir)

	resp, err := control.DeployWorkload(models.NodeSystemNamespace, Node1ServerPublicKey, gen.StartWorkloadRequestJson{
		Description:     "Test Workload",
		Argv:            []string{"--arg1", "value1"},
		Namespace:       models.NodeSystemNamespace,
		RetryCount:      3,
		Uri:             "file://" + binPath,
		WorkloadName:    "testworkload",
		WorkloadRuntype: "service",
		WorkloadType:    "direct-start",
		EncEnvironment: gen.SharedEncEnvJson{
			Base64EncryptedEnv: base64.StdEncoding.EncodeToString(encEnv),
			EncryptedBy:        tAPub,
		},
	})
	be.NilErr(t, err)

	be.True(t, resp.Started)

	aResp, err := control.Auction(models.NodeSystemNamespace, map[string]string{
		models.TagNodeName: "node-3",
	})
	be.NilErr(t, err)

	cResp, err := control.CopyWorkload(resp.Id, models.NodeSystemNamespace, aResp[0].TargetXkey)
	be.NilErr(t, err)

	be.Equal(t, "testworkload", cResp.WorkloadName)
	be.AllEqual(t, []string{"--arg1", "value1"}, cResp.Argv)
	be.Equal(t, Node1XkeyPublicKey, cResp.EncEnvironment.EncryptedBy)
}

func TestMonitorEndpoints(t *testing.T) {
	workingDir := t.TempDir()
	natsServer := startNatsServer(t, workingDir)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)

	t.Cleanup(func() {
		os.RemoveAll(filepath.Join(os.TempDir(), "inex-"+Node1ServerPublicKey))
		cancel()
		natsServer.Shutdown()
	})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	logger := slog.New(shandler.NewHandler(
		shandler.WithLogLevel(slog.LevelDebug),
		shandler.WithGroupFilter([]string{"actor_system"}),
		shandler.WithStdOut(stdout),
		shandler.WithStdErr(stderr),
	))

	startNexus(t, ctx, logger, workingDir, natsServer.ClientURL(), 1)
	time.Sleep(1000 * time.Millisecond)

	nc, err := nats.Connect(natsServer.ClientURL())
	be.NilErr(t, err)

	control, err := NewControlApiClient(nc, logger)
	be.NilErr(t, err)

	logs, err := control.MonitorLogs(models.NodeSystemNamespace, "*", "*")
	be.NilErr(t, err)

	logBuf := new(bytes.Buffer)
	go func() {
		for m := range logs {
			_, err := logBuf.Write(m)
			be.NilErr(t, err)
		}
	}()
	time.Sleep(250 * time.Millisecond)

	err = nc.Publish("$NEX.logs."+models.NodeSystemNamespace+".b.c", []byte("log test"))
	be.NilErr(t, err)
	time.Sleep(250 * time.Millisecond)
	close(logs)

	events, err := control.MonitorEvents(models.NodeSystemNamespace, "*", "*")
	be.NilErr(t, err)

	eventBuf := new(bytes.Buffer)
	go func() {
		for m := range events {
			mB, err := json.Marshal(m)
			be.NilErr(t, err)
			_, err = eventBuf.Write(mB)
			be.NilErr(t, err)
		}
	}()
	time.Sleep(250 * time.Millisecond)

	err = nc.Publish("$NEX.events."+models.NodeSystemNamespace+".b.c", []byte("{\"test\": \"event\"}"))
	be.NilErr(t, err)
	time.Sleep(250 * time.Millisecond)
	close(events)

	be.Equal(t, "log test", logBuf.String())
	be.Equal(t, "{\"test\":\"event\"}", eventBuf.String())
}
