package test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/nex/node"
)

const (
	Node1ServerSeed      string = "SNAB2T3VG2363NDA2JK7NT5O3FN5VCXI2MYJHOPFO2NIDXQU6DIWQTBQC4"
	Node1ServerPublicKey string = "NCUU2YIYXEPGTCDXDKQR7LL5PXDHIDG7SDFLWKE3WY63ZGCZL2HKIAJT"
	Node1XKeySeed        string = "SXAOUP7RZFW5QPE2GDWTPABUDM5UIAK6BPULJPWZQAFFL2RZ5K3UYWHYY4"
	Node1XkeyPublicKey   string = "XAL54S5FE6SRPONXRNVE4ZDAOHOT44GFIY2ZW33DHLR2U3H2HJSXXRKY"
)

func BuildTestBinary(t testing.TB, binMain string, workingDir string) string {
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

func StartNatsServer(t testing.TB, workingDir string) *server.Server {
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

func StartNexus(t testing.TB, ctx context.Context, logger *slog.Logger, workingDir, natsUrl string, numNodes int) {
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

// following helpers are for e2e tests in the test package only

func buildNexCli(t testing.TB, workingDir string) string {
	t.Helper()

	if _, err := os.Stat(filepath.Join(workingDir, "nex")); err == nil {
		return filepath.Join(workingDir, "nex")
	}

	cwd, err := os.Getwd()
	be.NilErr(t, err)

	err = os.Chdir("../cmd/nex")
	be.NilErr(t, err)

	cmd := exec.Command("go", "build", "-o", workingDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	be.NilErr(t, err)

	err = cmd.Wait()
	be.NilErr(t, err)

	err = os.Chdir(cwd)
	be.NilErr(t, err)

	return filepath.Join(workingDir, "nex")
}

func startNexNodeCmd(t testing.TB, workingDir, nodeSeed, xkeySeed, natsServer, name, nexus string) *exec.Cmd {
	t.Helper()

	cli := buildNexCli(t, workingDir)

	if nodeSeed == "" {
		kp, err := nkeys.CreateServer()
		be.NilErr(t, err)

		s, err := kp.Seed()
		be.NilErr(t, err)

		nodeSeed = string(s)
	}

	if xkeySeed == "" {
		xkp, err := nkeys.CreateCurveKeys()
		be.NilErr(t, err)

		xSeed, err := xkp.Seed()
		be.NilErr(t, err)

		xkeySeed = string(xSeed)
	}

	cmd := exec.Command(cli, "node", "up", "--logger.level", "debug", "--logger.short", "-s", natsServer, "--resource-directory", workingDir, "--node-name", name, "--nexus", nexus, "--node-seed", nodeSeed, "--node-xkey-seed", xkeySeed, "--start-message", "test workload started", "--stop-message", "test workload stopped")
	return cmd
}
