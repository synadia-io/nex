package spec

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/synadia-io/nex/internal/models"
	nexnode "github.com/synadia-io/nex/internal/node"
)

const defaultCNIPluginBinPath = "/opt/cni/bin"
const defaultCNIConfigurationPath = "/etc/cni/conf.d"
const defaultFirecrackerBinPath = "/usr/local/bin/firecracker"

var _ io.Reader = (*os.File)(nil)

var _ = Describe("nex node", func() {
	var log *slog.Logger
	var ctxx context.Context
	var cancel context.CancelFunc

	var opts *models.Options
	var nodeOpts *models.NodeOptions

	var nodeConfig nexnode.NodeConfiguration

	var validResourceDir string // prevent downloading kernel and rootfs template multiple times
	var validResourceDirOnce sync.Once

	var snapshotAgentRootFSPath string
	var snapshotAgentRootFSPathOnce sync.Once

	BeforeEach(func() {
		ctxx, cancel = context.WithCancel(context.Background())
		log = slog.New(slog.NewTextHandler(os.Stdout, nil))

		opts = &models.Options{
			Servers: _fixtures.natsServer.ClientURL(),
		}
		nodeOpts = &models.NodeOptions{}

		_ = os.MkdirAll(defaultCNIPluginBinPath, 0755)
		_ = os.MkdirAll(defaultCNIConfigurationPath, 0755)

		validResourceDirOnce.Do(func() {
			validResourceDir = filepath.Join(os.TempDir(), fmt.Sprintf("%d-spec-nex-wd", _fixtures.seededRand.Int()))
		})

		snapshotAgentRootFSPathOnce.Do(func() {
			// make sure dagger isn't already running, as this has been known to cause problems...
			stopDaggerEngine()

			snapshotAgentRootFSPath = filepath.Join(os.TempDir(), fmt.Sprintf("%d-rootfs.ext4", _fixtures.seededRand.Int()))
			cmd := exec.Command("go", "run", "../agent/fc-image", "../agent/agent.go")
			_, _ = cmd.Output()

			compressedRootFS, _ := os.Open("./rootfs.ext4.gz")
			uncompressedRootFS, _ := gzip.NewReader(bufio.NewReader(compressedRootFS))

			outFile, _ := os.Create(snapshotAgentRootFSPath)
			defer outFile.Close()

			_, _ = io.Copy(outFile, uncompressedRootFS)
			os.Remove("./rootfs.ext4.gz")
		})
	})

	Describe("preflight", func() {
		Context("when the specified configuration file does not exist", func() {
			BeforeEach(func() {
				nodeOpts.ConfigFilepath = filepath.Join(os.TempDir(), fmt.Sprintf("%d-non-existent-nex-conf.json", _fixtures.seededRand.Int()))
			})

			It("should return an error", func(ctx SpecContext) {
				Expect(
					nexnode.CmdPreflight(opts, nodeOpts, ctxx, cancel, log),
				).To(MatchError(fmt.Errorf("failed to load configuration file: open %s: no such file or directory", nodeOpts.ConfigFilepath)))
			})
		})

		Context("when the specified node configuration file exists", func() {
			BeforeEach(func() {
				nodeConfig = nexnode.DefaultNodeConfiguration()
				nodeOpts.ConfigFilepath = path.Join(os.TempDir(), fmt.Sprintf("%d-spec-nex-conf.json", _fixtures.seededRand.Int()))
			})

			JustBeforeEach(func() {
				cfg, _ := json.Marshal(nodeConfig)
				_ = os.WriteFile(nodeOpts.ConfigFilepath, cfg, 0644)
			})

			AfterEach(func() {
				os.Remove(nodeOpts.ConfigFilepath)
			})

			Describe("default dependency resolution", func() {
				Context("when the node configuration specifies a default_resource_dir", func() {
					Context("when the specified default_resource_dir does not exist on the host", func() {
						BeforeEach(func() {
							nodeConfig.DefaultResourceDir = filepath.Join(os.TempDir(), fmt.Sprintf("%d-non-existent-nex-resource-dir", _fixtures.seededRand.Int()))
						})

						It("should return an error", func(ctx SpecContext) {
							Expect(
								nexnode.CmdPreflight(opts, nodeOpts, ctxx, cancel, log),
							).To(MatchError(errors.New("preflight checks failed: EOF")))
						})
					})

					Context("when the specified default_resource_dir exists on the host", func() {
						BeforeEach(func() {
							nodeConfig.DefaultResourceDir = validResourceDir
							_ = os.Mkdir(nodeConfig.DefaultResourceDir, 0755)
							nodeOpts.ForceDepInstall = true
						})

						JustBeforeEach(func() {
							_ = nexnode.CmdPreflight(opts, nodeOpts, ctxx, cancel, log)
						})

						It("should install the host-local CNI plugin", func(ctx SpecContext) {
							_, err := os.Stat(filepath.Join(defaultCNIPluginBinPath, "host-local"))
							Expect(err).To(BeNil())
						})

						It("should install the ptp CNI plugin", func(ctx SpecContext) {
							_, err := os.Stat(filepath.Join(defaultCNIPluginBinPath, "ptp"))
							Expect(err).To(BeNil())
						})

						It("should install the tc-redirect-tap CNI plugin", func(ctx SpecContext) {
							_, err := os.Stat(filepath.Join(defaultCNIPluginBinPath, "tc-redirect-tap"))
							Expect(err).To(BeNil())
						})

						It("should install the default firecracker binary", func(ctx SpecContext) {
							_, err := os.Stat(defaultFirecrackerBinPath)
							Expect(err).To(BeNil())
						})

						It("should install the default CNI configuration", func(ctx SpecContext) {
							_, err := os.Stat(filepath.Join(defaultCNIConfigurationPath, fmt.Sprintf("%s.conflist", *nodeConfig.CNI.NetworkName)))
							Expect(err).To(BeNil())
						})

						It("should fetch the default vmlinux kernel", func(ctx SpecContext) {
							_, err := os.Stat(filepath.Join(nodeConfig.DefaultResourceDir, "vmlinux"))
							Expect(err).To(BeNil())
						})

						It("should fetch the default agent rootfs template", func(ctx SpecContext) {
							_, err := os.Stat(filepath.Join(nodeConfig.DefaultResourceDir, "rootfs.ext4"))
							Expect(err).To(BeNil())
						})
					})
				})
			})
		})
	})

	Describe("up", func() {
		Context("when the specified configuration file does not exist", func() {
			BeforeEach(func() {
				nodeOpts.ConfigFilepath = filepath.Join(os.TempDir(), fmt.Sprintf("%d-non-existent-nex-conf.json", _fixtures.seededRand.Int()))
			})

			It("should return an error", func(ctx SpecContext) {
				err := nexnode.CmdUp(opts, nodeOpts, ctxx, cancel, log)
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(ContainSubstring("failed to load node configuration file"))
			})
		})

		Context("when the specified node configuration file exists", func() {
			BeforeEach(func() {
				nodeConfig = nexnode.DefaultNodeConfiguration()
				nodeOpts.ConfigFilepath = path.Join(os.TempDir(), fmt.Sprintf("%d-spec-nex-conf.json", _fixtures.seededRand.Int()))
			})

			JustBeforeEach(func() {
				cfg, _ := json.Marshal(nodeConfig)
				os.WriteFile(nodeOpts.ConfigFilepath, cfg, 0644)
			})

			AfterEach(func() {
				os.Remove(nodeOpts.ConfigFilepath)
			})

			Context("when the node configuration specifies a default_resource_dir", func() {
				Context("when the specified default_resource_dir does not exist on the host", func() {
					BeforeEach(func() {
						nodeConfig.DefaultResourceDir = filepath.Join(os.TempDir(), fmt.Sprintf("%d-non-existent-nex-resource-dir", _fixtures.seededRand.Int()))
					})

					It("should return an error", func(ctx SpecContext) {
						err := nexnode.CmdUp(opts, nodeOpts, ctxx, cancel, log)
						Expect(err).ToNot(BeNil())
						Expect(err.Error()).To(ContainSubstring("failed to start node"))
					})
				})

				Context("when the specified default_resource_dir exists on the host", func() {
					var node *nexnode.Node

					BeforeEach(func() {
						nodeConfig.DefaultResourceDir = validResourceDir
						os.Mkdir(validResourceDir, 0755)
						nodeOpts.ForceDepInstall = true
					})

					AfterEach(func() {
						node.Stop()
					})

					JustBeforeEach(func() {
						nexnode.CmdPreflight(opts, nodeOpts, ctxx, cancel, log)
						node, _ = nexnode.NewNode(opts, nodeOpts, ctxx, cancel, log)
						go node.Start()

						time.Sleep(time.Millisecond * 500)
					})

					It("should initialize the node API", func(ctx SpecContext) {
						subsz, _ := _fixtures.natsServer.Subsz(&server.SubszOptions{Subscriptions: true})
						fmt.Printf("%v", subsz)
					})

					It("should initialize a machine manager instance", func(ctx SpecContext) {

					})

					It("should install signal handlers", func(ctx SpecContext) {

					})

					It("should use the provided logger instance", func(ctx SpecContext) {

					})

					It("should initialize the node configuration", func(ctx SpecContext) {

					})

					It("should hold a reference to the given options", func(ctx SpecContext) {

					})

					It("should hold a reference to the given node options", func(ctx SpecContext) {

					})

					It("should establish a connection with the configured NATS server", func(ctx SpecContext) {

					})

					It("should initialize an internal NATS server for private communication between running VMs and the host", func(ctx SpecContext) {

					})
				})
			})
		})
	})
})
