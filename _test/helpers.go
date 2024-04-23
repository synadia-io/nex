package test

// import (
// 	"bufio"
// 	"compress/gzip"
// 	"context"
// 	"errors"
// 	"fmt"
// 	"io"
// 	"math/rand"
// 	"net/url"
// 	"os"
// 	"os/exec"
// 	"path"
// 	"path/filepath"
// 	"strconv"
// 	"time"

// 	"github.com/docker/docker/api/types/container"
// 	"github.com/docker/docker/api/types/filters"
// 	"github.com/docker/docker/client"
// 	"github.com/nats-io/nats-server/v2/server"
// 	"github.com/nats-io/nats.go"
// )

// var (
// 	seededRand              = rand.New(rand.NewSource(time.Now().UnixNano()))
// 	snapshotAgentRootFSPath string
// )

// func startTestNATS() (*server.Server, *nats.Conn, int, error) {
// 	ns, err := server.NewServer(&server.Options{
// 		Host:      "0.0.0.0",
// 		Port:      -1,
// 		JetStream: true,
// 		NoLog:     true,
// 		StoreDir:  path.Join(os.TempDir(), fmt.Sprintf("%d-integtest", seededRand.Int())),
// 	})
// 	if err != nil {
// 		return nil, nil, 0, err
// 	}
// 	ns.Start()

// 	clientURL, err := url.Parse(ns.ClientURL())
// 	if err != nil {
// 		return nil, nil, 0, fmt.Errorf("failed to parse internal NATS client URL: %s", err)
// 	}

// 	port, err := strconv.Atoi(clientURL.Port())
// 	if err != nil {
// 		return nil, nil, 0, fmt.Errorf("failed to parse internal NATS client URL: %s", err)
// 	}

// 	nc, err := nats.Connect(ns.ClientURL())
// 	if err != nil {
// 		return nil, nil, 0, fmt.Errorf("failed to connect to NATS server: %s", err)
// 	}

// 	js, err := nc.JetStream()
// 	if err != nil {
// 		return nil, nil, 0, fmt.Errorf("failed to resolve jetstream context via NATS connection: %s", err)
// 	}

// 	_, err = js.CreateObjectStore(&nats.ObjectStoreConfig{
// 		Bucket:      "NEXCLIFILES",
// 		Description: "Client-facing cache for deploying workloads",
// 		Storage:     nats.MemoryStorage,
// 	})
// 	if err != nil {
// 		return nil, nil, 0, fmt.Errorf("failed to create jetstream object store: %s", err)
// 	}

// 	return ns, nc, port, nil
// }

// func loadPreflight() {
// 	// make sure dagger isn't already running, as this has been known to cause problems...
// 	stopDaggerEngine()

// 	// require the nex-agent binary to be built... FIXME-- build it here insteaad of relying on the Taskfile
// 	_, err := os.Stat("../agent/cmd/nex-agent/nex-agent")
// 	if err != nil {
// 		panic(err)
// 	}

// 	agentPath, err := filepath.Abs("../agent/cmd/nex-agent")
// 	if err != nil {
// 		panic(err)
// 	}

// 	os.Setenv("PATH", fmt.Sprintf("%s:%s", os.Getenv("PATH"), agentPath))

// 	snapshotAgentRootFSPath = filepath.Join(os.TempDir(), fmt.Sprintf("%d-rootfs.ext4", seededRand.Int()))
// 	buildRootFs()

// 	compressedRootFS, err := os.Open("./rootfs.ext4.gz")
// 	if err != nil {
// 		panic(err)
// 	}

// 	uncompressedRootFS, err := gzip.NewReader(bufio.NewReader(compressedRootFS))
// 	if err != nil {
// 		panic(err)
// 	}

// 	outFile, _ := os.Create(snapshotAgentRootFSPath)
// 	defer outFile.Close()

// 	_, _ = io.Copy(outFile, uncompressedRootFS)
// 	_ = os.Remove("./rootfs.ext4.gz")
// }

// func stopDaggerEngine() {
// 	cli, err := client.NewClientWithOpts(client.FromEnv)
// 	if err != nil {
// 		return
// 	}

// 	ctx := context.Background()

// 	containers, _ := cli.ContainerList(ctx, container.ListOptions{
// 		All: true,
// 		Filters: filters.NewArgs([]filters.KeyValuePair{
// 			{
// 				Key:   "name",
// 				Value: "dagger-engine",
// 			},
// 		}...),
// 	})

// 	for _, c := range containers {
// 		_ = cli.ContainerStop(ctx, c.ID, container.StopOptions{})
// 	}
// }

// // separate function so we can build-tag guard this
// func buildRootFs() {
// 	cmd := exec.Command("sudo", "go", "run", "../nex", "fs", "--agent", "../agent/cmd/nex-agent/nex-agent")
// 	_, err := cmd.CombinedOutput()
// 	if err != nil {
// 		panic(err)
// 	}
// 	if cmd.ProcessState.ExitCode() != 0 {
// 		panic(errors.New("Failed to execute go run nex fs"))
// 	}
// }
