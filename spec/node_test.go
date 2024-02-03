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
	"os/signal"
	"path"
	"path/filepath"
	"sync"
	"syscall"
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

			// require the nex-agent binary to be built... FIXME-- build it here insteaad of relying on the Taskfile
			_, err := os.Stat("../agent/cmd/nex-agent/nex-agent")
			Expect(err).To(BeNil())

			snapshotAgentRootFSPath = filepath.Join(os.TempDir(), fmt.Sprintf("%d-rootfs.ext4", _fixtures.seededRand.Int()))
			cmd := exec.Command("go", "run", "../agent/fc-image", "../agent/cmd/nex-agent/nex-agent")
			_ = cmd.Start()
			_ = cmd.Wait()

			compressedRootFS, _ := os.Open("./rootfs.ext4.gz")
			uncompressedRootFS, _ := gzip.NewReader(bufio.NewReader(compressedRootFS))

			outFile, _ := os.Create(snapshotAgentRootFSPath)
			defer outFile.Close()

			_, _ = io.Copy(outFile, uncompressedRootFS)
			_ = os.Remove("./rootfs.ext4.gz")
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
				_ = os.WriteFile(nodeOpts.ConfigFilepath, cfg, 0644)
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
					var nodeProxy *nexnode.NodeProxy
					var nodeID *string // node id == node public key

					BeforeEach(func() {
						nodeConfig.DefaultResourceDir = validResourceDir
						nodeConfig.RootFsFilepath = snapshotAgentRootFSPath
						_ = os.Mkdir(validResourceDir, 0755)
						nodeOpts.ForceDepInstall = true
					})

					AfterEach(func() {
						node.Stop()
					})

					JustBeforeEach(func() {
						_ = nexnode.CmdPreflight(opts, nodeOpts, ctxx, cancel, log)
						node, _ = nexnode.NewNode(opts, nodeOpts, ctxx, cancel, log)
						go node.Start()

						nodeID, _ = node.PublicKey()
						nodeProxy = nexnode.NewNodeProxyWith(node)
						time.Sleep(time.Millisecond * 500)
					})

					It("should generate a keypair for the node", func(ctx SpecContext) {
						publicKey, err := node.PublicKey()
						Expect(err).To(BeNil())
						Expect(publicKey).ToNot(BeNil())
					})

					It("should install signal handlers", func(ctx SpecContext) {
						Expect(signal.Ignored(os.Interrupt)).To(BeFalse())
						Expect(signal.Ignored(syscall.SIGTERM)).To(BeFalse())
						Expect(signal.Ignored(syscall.SIGINT)).To(BeFalse())
						Expect(signal.Ignored(syscall.SIGQUIT)).To(BeFalse())
					})

					It("should use the provided logger instance", func(ctx SpecContext) {
						Expect(nodeProxy.Log()).To(Equal(log))
					})

					It("should initialize the node configuration", func(ctx SpecContext) {
						Expect(nodeProxy.NodeConfiguration()).ToNot(BeNil()) // FIXME-- assert that it is === to the current nex node config JSON
					})

					It("should initialize an internal NATS server for private communication between running VMs and the host", func(ctx SpecContext) {
						Expect(nodeProxy.InternalNATS()).ToNot(BeNil())
					})

					It("should initialize a machine manager to manage firecracker VMs and communicate with running agents", func(ctx SpecContext) {
						Expect(nodeProxy.MachineManager()).ToNot(BeNil())
					})

					It("should initialize an API listener", func(ctx SpecContext) {
						Expect(nodeProxy.APIListener()).ToNot(BeNil())
					})

					// FIXME-- this needs to be updated
					// It("should initialize a telemetry instance", func(ctx SpecContext) {
					// 	Expect(nodeProxy.Telemetry()).ToNot(BeNil())
					// })

					Context("when node options enable otel", func() {
						// TODO
					})

					Describe("node API listener subscriptions", func() {
						It("should initialize a node API subscription for handling ping requests", func(ctx SpecContext) {
							subsz, _ := _fixtures.natsServer.Subsz(&server.SubszOptions{
								Subscriptions: true,
								Test:          "$NEX.PING",
							})
							Expect(subsz.Total).To(Equal(1))
						})

						It("should initialize a node API subscription for handling ping requests with a specific node id", func(ctx SpecContext) {
							subsz, _ := _fixtures.natsServer.Subsz(&server.SubszOptions{
								Subscriptions: true,
								Test:          fmt.Sprintf("$NEX.PING.%s", *nodeID),
							})
							Expect(subsz.Total).To(Equal(1))
						})

						It("should initialize a node API subscription for handling namespaced info requests", func(ctx SpecContext) {
							subsz, _ := _fixtures.natsServer.Subsz(&server.SubszOptions{
								Subscriptions: true,
								Test:          fmt.Sprintf("$NEX.INFO.default.%s", *nodeID),
							})
							Expect(subsz.Total).To(Equal(1))
						})

						It("should initialize a node API subscription for handling namespaced deploy requests", func(ctx SpecContext) {
							subsz, _ := _fixtures.natsServer.Subsz(&server.SubszOptions{
								Subscriptions: true,
								Test:          fmt.Sprintf("$NEX.DEPLOY.default.%s", *nodeID),
							})
							Expect(subsz.Total).To(Equal(1))
						})

						It("should initialize a node API subscription for handling namespaced workload stop -- FIXME? should be undeploy?", func(ctx SpecContext) {
							subsz, _ := _fixtures.natsServer.Subsz(&server.SubszOptions{
								Subscriptions: true,
								Test:          fmt.Sprintf("$NEX.STOP.default.%s", *nodeID),
							})
							Expect(subsz.Total).To(Equal(1))
						})
					})

					Describe("machine manager", func() {
						var manager *nexnode.MachineManager

						JustBeforeEach(func() {
							nodeConfig.DefaultResourceDir = validResourceDir
							_ = os.Mkdir(validResourceDir, 0755)
							nodeOpts.ForceDepInstall = true

							manager = nodeProxy.MachineManager()
							fmt.Printf("%v", manager) // HACK-- remove once we use manager for assertions... this is just here to remove the unused warning for now...

							time.Sleep(time.Millisecond * 2500) // allow enough time for the pool to warm up...
						})

						Describe("agent internal API subscriptions", func() {
							It("should initialize an internal API subscription for handling agent handshake requests", func(ctx SpecContext) {
								subsz, _ := nodeProxy.InternalNATS().Subsz(&server.SubszOptions{
									Subscriptions: true,
									Test:          "agentint.handshake",
								})
								Expect(subsz.Total).To(Equal(1))
							})

							It("should initialize an internal API subscription for handling agent events", func(ctx SpecContext) {
								subsz, _ := nodeProxy.InternalNATS().Subsz(&server.SubszOptions{
									Subscriptions: true,
									Test:          "agentint.vmid.events.event_type",
								})
								Expect(subsz.Total).To(Equal(1))
							})

							It("should initialize an internal API subscription for handling agent logs", func(ctx SpecContext) {
								subsz, _ := nodeProxy.InternalNATS().Subsz(&server.SubszOptions{
									Subscriptions: true,
									Test:          "agentint.vmid.logs",
								})
								Expect(subsz.Total).To(Equal(1))
							})
						})

						Describe("VM pool", func() {
							Context("when a VM is warm", func() {
								It("should complete the agent handshake", func(ctx SpecContext) {
									subsz, _ := nodeProxy.InternalNATS().Subsz(&server.SubszOptions{
										Subscriptions: true,
										Test:          "agentint.handshake",
									})
									Expect(subsz.Subs[0].Msgs).To(Equal(int64(1)))
								})
							})
						})
					})
				})
			})
		})
	})
})
