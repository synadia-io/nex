package nodecontrol

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"disorder.dev/shandler"
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

	err := cmd.Run()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(binPath); err != nil {
		t.Fatal(err)
	}

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
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < numNodes; i++ {
		var kp, xkp nkeys.KeyPair
		if i == 0 {
			kp, err = nkeys.FromSeed([]byte(Node1ServerSeed))
			if err != nil {
				t.Fatal(err)
			}
			xkp, err = nkeys.FromSeed([]byte(Node1XKeySeed))
			if err != nil {
				t.Fatal(err)
			}
		} else {
			kp, err = nkeys.CreateServer()
			if err != nil {
				t.Fatal(err)
			}
			xkp, err = nkeys.CreateCurveKeys()
			if err != nil {
				t.Fatal(err)
			}
		}
		nn, err := node.NewNexNode(kp, nc,
			models.WithContext(ctx),
			models.WithLogger(logger),
			models.WithXKeyKeyPair(xkp),
			models.WithNodeName(fmt.Sprintf("node-%d", i+1)),
			models.WithNexus("testnexus"),
			models.WithResourceDirectory(workingDir),
		)
		if err != nil {
			t.Fatal(err)
		}

		err = nn.Validate()
		if err != nil {
			t.Fatal(err)
		}

		go func() {
			err = nn.Start()
			if err != nil {
				t.Error(err)
				t.FailNow()
			}
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
	if err != nil {
		t.Fatal(err)
	}

	control, err := NewControlApiClient(nc, logger)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := control.Auction(models.NodeSystemNamespace, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}

	if len(resp) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resp))
	}

	resp, err = control.Auction(models.NodeSystemNamespace, map[string]string{models.TagNodeName: "notreal"})
	if err != nil {
		t.Fatal(err)
	}

	if len(resp) != 0 {
		t.Fatalf("expected 0 response, got %d", len(resp))
	}
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
	if err != nil {
		t.Fatal(err)
	}

	control, err := NewControlApiClient(nc, logger)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := control.Ping()
	if err != nil {
		t.Fatal(err)
	}

	if len(resp) != 5 {
		t.Fatalf("expected 5 responses, got %d", len(resp))
	}
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
	if err != nil {
		t.Fatal(err)
	}

	control, err := NewControlApiClient(nc, logger)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := control.DirectPing(Node1ServerPublicKey)
	if err != nil {
		t.Fatal(err)
	}

	if resp.NodeId != Node1ServerPublicKey {
		t.Fatalf("expected node id %s, got %s", Node1ServerPublicKey, resp.NodeId)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	control, err := NewControlApiClient(nc, logger)
	if err != nil {
		t.Fatal(err)
	}

	auctionResp, err := control.Auction(models.NodeSystemNamespace, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}

	env := make(map[string]string)
	envB, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	tAKey, err := nkeys.CreateCurveKeys()
	if err != nil {
		t.Fatal(err)
	}

	tAPub, err := tAKey.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	encEnv, err := tAKey.Seal(envB, auctionResp[0].TargetXkey)
	if err != nil {
		t.Fatal(err)
	}

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
	if err != nil {
		t.Fatal(err)
	}

	if !resp.Started {
		t.Fatalf("expected workload to be started")
	}

	pingResp, err := control.FindWorkload(models.NodeSystemNamespace, resp.Id)
	if err != nil {
		t.Fatal(err)
	}

	if pingResp.WorkloadSummary.Id != resp.Id {
		t.Fatalf("expected workload id %s, got %s", resp.Id, pingResp.WorkloadSummary.Id)
	}

	if pingResp.WorkloadSummary.WorkloadState != models.WorkloadStateRunning {
		t.Fatalf("expected workload status running, got %s", pingResp.WorkloadSummary.WorkloadState)
	}

	_, err = control.FindWorkload("badnamespace", resp.Id)
	if !errors.Is(err, nats.ErrTimeout) {
		t.Fatalf("expected timeout error, got %v", err)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	control, err := NewControlApiClient(nc, logger)
	if err != nil {
		t.Fatal(err)
	}

	env := make(map[string]string)
	envB, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	tAKey, err := nkeys.CreateCurveKeys()
	if err != nil {
		t.Fatal(err)
	}

	tAPub, err := tAKey.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	encEnv, err := tAKey.Seal(envB, Node1XkeyPublicKey)
	if err != nil {
		t.Fatal(err)
	}

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
	if err != nil {
		t.Fatal(err)
	}

	if !resp.Started {
		t.Fatalf("expected workload to be started")
	}

	wl, err := control.ListWorkloads(models.NodeSystemNamespace)
	if err != nil {
		t.Fatal(err)
	}

	if len(wl) != 1 {
		t.Fatalf("expected 1 workload, got %d", len(wl))
	}

	wl, err = control.ListWorkloads("badnamespace")
	if err != nil {
		t.Fatal(err)
	}

	if len(wl) != 0 {
		t.Fatalf("expected 0 workloads, got %d", len(wl))
	}
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
	if err != nil {
		t.Fatal(err)
	}

	control, err := NewControlApiClient(nc, logger)
	if err != nil {
		t.Fatal(err)
	}

	env := make(map[string]string)
	envB, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	tAKey, err := nkeys.CreateCurveKeys()
	if err != nil {
		t.Fatal(err)
	}

	tAPub, err := tAKey.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	encEnv, err := tAKey.Seal(envB, Node1XkeyPublicKey)
	if err != nil {
		t.Fatal(err)
	}

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
	if err != nil {
		t.Fatal(err)
	}

	if !resp.Started {
		t.Fatalf("expected workload to be started")
	}

	stopResp, err := control.UndeployWorkload(models.NodeSystemNamespace, resp.Id)
	if err != nil {
		t.Fatal(err)
	}

	if !stopResp.Stopped {
		t.Fatal("expected workload to be stopped")
	}
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
	if err != nil {
		t.Fatal(err)
	}

	control, err := NewControlApiClient(nc, logger)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := control.GetInfo(Node1ServerPublicKey, gen.NodeInfoRequestJson{
		Namespace: models.NodeSystemNamespace,
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.NodeId != Node1ServerPublicKey {
		t.Fatalf("expected node id %s, got %s", Node1ServerPublicKey, resp.NodeId)
	}

	if resp.Tags.Tags[models.TagNodeName] != "node-1" {
		t.Fatalf("expected node name node-1, got %s", resp.Tags.Tags[models.TagNodeName])
	}

	if resp.Tags.Tags[models.TagNexus] != "testnexus" {
		t.Fatalf("expected nexus testnexus, got %s", resp.Tags.Tags[models.TagNexus])
	}

	if len(resp.WorkloadSummaries) != 0 {
		t.Fatalf("expected 0 workloads, got %d", len(resp.WorkloadSummaries))
	}
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
	if err != nil {
		t.Fatal(err)
	}

	control, err := NewControlApiClient(nc, logger)
	if err != nil {
		t.Fatal(err)
	}

	_, err = control.SetLameDuck(Node1ServerPublicKey, time.Second*3)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := control.GetInfo(Node1ServerPublicKey, gen.NodeInfoRequestJson{
		Namespace: models.NodeSystemNamespace,
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Tags.Tags[models.TagLameDuck] != "true" {
		t.Fatalf("expected lameduck true, got %s", resp.Tags.Tags[models.TagLameDuck])
	}
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
	if err != nil {
		t.Fatal(err)
	}

	control, err := NewControlApiClient(nc, logger)
	if err != nil {
		t.Fatal(err)
	}

	env := make(map[string]string)
	envB, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	tAKey, err := nkeys.CreateCurveKeys()
	if err != nil {
		t.Fatal(err)
	}

	tAPub, err := tAKey.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	encEnv, err := tAKey.Seal(envB, Node1XkeyPublicKey)
	if err != nil {
		t.Fatal(err)
	}

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
	if err != nil {
		t.Fatal(err)
	}

	if !resp.Started {
		t.Fatalf("expected workload to be started")
	}

	aResp, err := control.Auction(models.NodeSystemNamespace, map[string]string{
		models.TagNodeName: "node-3",
	})
	if err != nil {
		t.Fatal(err)
	}

	cResp, err := control.CopyWorkload(resp.Id, models.NodeSystemNamespace, aResp[0].TargetXkey)
	if err != nil {
		t.Fatal(err)
	}

	if cResp.WorkloadName != "testworkload" {
		t.Fatalf("expected workload name testworkload, got %s", cResp.WorkloadName)
	}

	if len(cResp.Argv) != 2 && cResp.Argv[0] != "--arg1" && cResp.Argv[1] != "value1" {
		t.Fatalf("expected arg1 value --arg1, got %s", cResp.Argv[0])
	}

	if cResp.EncEnvironment.EncryptedBy != Node1XkeyPublicKey {
		t.Fatalf("expected workload encrypted by %s, got %s", Node1XkeyPublicKey, cResp.EncEnvironment.EncryptedBy)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	control, err := NewControlApiClient(nc, logger)
	if err != nil {
		t.Fatal(err)
	}

	logs, err := control.MonitorLogs(models.NodeSystemNamespace, "*", "*")
	if err != nil {
		t.Fatal(err)
	}

	logBuf := new(bytes.Buffer)
	go func() {
		for m := range logs {
			_, err := logBuf.Write(m)
			if err != nil {
				t.Error(err)
			}
		}
	}()

	time.Sleep(250 * time.Millisecond)
	err = nc.Publish("$NEX.logs."+models.NodeSystemNamespace+".b.c", []byte("log test"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	close(logs)

	// ----
	events, err := control.MonitorEvents(models.NodeSystemNamespace, "*", "*")
	if err != nil {
		t.Fatal(err)
	}

	eventBuf := new(bytes.Buffer)
	go func() {
		for m := range events {
			mB, err := json.Marshal(m)
			if err != nil {
				t.Error(err)
			}
			_, err = eventBuf.Write(mB)
			if err != nil {
				t.Error(err)
			}
		}
	}()

	time.Sleep(250 * time.Millisecond)
	err = nc.Publish("$NEX.events."+models.NodeSystemNamespace+".b.c", []byte("{\"test\": \"event\"}"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	close(events)

	if logBuf.String() != "log test" {
		t.Fatalf("expected log test, got %s", string(logBuf.String()))
	}

	if eventBuf.String() != "{\"test\":\"event\"}" {
		t.Fatalf("expected event {\"test\":\"event\"}, got %s", eventBuf.String())
	}
}
