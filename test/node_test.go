package test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/nats-io/jsm.go/natscontext"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/models"
	nexnode "github.com/synadia-io/nex/internal/node"
)

var (
	testServer       *server.Server
	testConn         *nats.Conn
	testNode         *nexnode.Node
	testPort         int
	nodeOpts         *models.NodeOptions
	log              *slog.Logger
	validResourceDir = filepath.Join(os.TempDir(), fmt.Sprintf("%d-test-nex-wd", seededRand.Int()))
)

const (
	defaultCNIPluginBinPath     = "/opt/cni/bin"
	defaultCNIConfigurationPath = "/etc/cni/conf.d"
	defaultFirecrackerBinPath   = "/usr/local/bin/firecracker"
)

func TestMain(m *testing.M) {
	fmt.Println("Starting node tests")
	//setup()
	m.Run()
	//teardown()
}

func TestPlaceholder(t *testing.T) {
	t.Log("placeholder")
}

func setup() {
	fmt.Println("Node test setup")
	log = slog.Default()

	var err error
	testServer, testConn, testPort, err = startTestNATS()
	if err != nil {
		panic(err)
	}

	createTestNode()

}

func teardown() {
	fmt.Println("Node test teardown")

	os.Remove(nodeOpts.ConfigFilepath)

	_ = testConn.Drain()
	testServer.Shutdown()
	testNode.Stop()
}

func createTestNode() {
	_ = os.MkdirAll(defaultCNIPluginBinPath, 0755)
	_ = os.MkdirAll(defaultCNIConfigurationPath, 0755)

	nodeOpts = &models.NodeOptions{}

	nodeConfig := models.DefaultNodeConfiguration()
	nodeConfig.DefaultResourceDir = validResourceDir
	nodeOpts.ConfigFilepath = path.Join(os.TempDir(), fmt.Sprintf("%d-test-nex-conf.json", seededRand.Int()))

	cfg, _ := json.Marshal(nodeConfig)
	_ = os.WriteFile(nodeOpts.ConfigFilepath, cfg, 0644)

	var err error
	ctx, cancelF := context.WithCancel(context.Background())
	testNode, err = nexnode.NewNode(
		&models.Options{
			Servers: testConn.ConnectedUrl(),

			Timeout:              5_000,
			ConnectionName:       "node-test",
			LogLevel:             "debug",
			LogJSON:              false,
			ConfigurationContext: "",
			Configuration:        &natscontext.Context{},
			SkipContexts:         true,
		},
		nodeOpts,
		ctx,
		cancelF,
		log,
	)
	if err != nil {
		panic(err)
	}

	testNode.Start()
}
