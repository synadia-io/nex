//go:build windows

package spec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	shandler "github.com/jordan-rash/slog-handler"
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"github.com/synadia-io/nex/internal/models"
	nexnode "github.com/synadia-io/nex/internal/node"
)

var _ io.Reader = (*os.File)(nil)

var _ = Describe("nex node", func() {
	var log *slog.Logger
	var ctxx context.Context
	var cancel context.CancelFunc

	var opts *models.Options
	var nodeOpts *models.NodeOptions
	var nodeKey nkeys.KeyPair

	var nodeConfig models.NodeConfiguration

	var validResourceDir string // prevent downloading kernel and rootfs template multiple times
	var validResourceDirOnce sync.Once

	var snapshotAgentRootFSPath string
	var snapshotAgentRootFSPathOnce sync.Once

	BeforeEach(func() {
		initData := map[string]string{
			"version":    "spec",
			"commit":     "abc123",
			"build_date": "2021-01-01T00:00:00Z",
		}

		ctxx, cancel = context.WithCancel(context.WithValue(context.Background(), "build_data", initData)) //nolint:all
		log = slog.New(shandler.NewHandler(shandler.WithLogLevel(slog.LevelDebug), shandler.WithColor()))

		opts = &models.Options{
			Servers: _fixtures.natsServer.ClientURL(),
		}
		nodeOpts = &models.NodeOptions{}

		validResourceDirOnce.Do(func() {
			validResourceDir = filepath.Join(os.TempDir(), fmt.Sprintf("%d-spec-nex-wd", _fixtures.seededRand.Int()))
		})

		snapshotAgentRootFSPathOnce.Do(func() {
			// require the nex-agent binary to be built... FIXME-- build it here insteaad of relying on the Taskfile
			_, err := os.Stat(filepath.Join("..", "agent", "cmd", "nex-agent", "nex-agent.exe"))
			Expect(err).To(BeNil())

			agentPath, err := filepath.Abs(filepath.Join("..", "agent", "cmd", "nex-agent"))
			Expect(err).To(BeNil())

			_ = os.Setenv("PATH", fmt.Sprintf("%s;%s", os.Getenv("PATH"), agentPath))
		})
	})

	Describe("preflight", func() {
		Context("when the specified configuration file does not exist", func() {
			BeforeEach(func() {
				nodeOpts.ConfigFilepath = filepath.Join(os.TempDir(), fmt.Sprintf("%d-non-existent-nex-conf.json", _fixtures.seededRand.Int()))
				nodeConfig.NoSandbox = true
				nodeConfig.WorkloadTypes = []controlapi.NexWorkload{controlapi.NexWorkloadNative, controlapi.NexWorkloadV8, controlapi.NexWorkloadWasm}
			})

			It("should not return an error", func(ctx SpecContext) {
				err := nexnode.CmdPreflight(opts, nodeOpts, ctxx, cancel, log)
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("open %s: The system cannot find the file specified", nodeOpts.ConfigFilepath)))
			})
		})

		Context("when the specified node configuration file exists", func() {
			BeforeEach(func() {
				nodeConfig = models.DefaultNodeConfiguration()
				nodeConfig.NoSandbox = true
				nodeOpts.ConfigFilepath = path.Join(os.TempDir(), fmt.Sprintf("%d-spec-nex-conf.json", _fixtures.seededRand.Int()))
				nodeConfig.WorkloadTypes = []controlapi.NexWorkload{controlapi.NexWorkloadNative, controlapi.NexWorkloadV8, controlapi.NexWorkloadWasm}
			})

			JustBeforeEach(func() {
				cfg, _ := json.Marshal(nodeConfig)
				_ = os.WriteFile(nodeOpts.ConfigFilepath, cfg, 0644)
			})

			AfterEach(func() {
				os.Remove(nodeOpts.ConfigFilepath)
			})

			Describe("ignoring dependency resolution required for sandbox mode", func() {
				Context("when the node configuration specifies a default_resource_dir", func() {
					Context("when the specified default_resource_dir does not exist on the host", func() {
						BeforeEach(func() {
							nodeConfig.DefaultResourceDir = filepath.Join(os.TempDir(), fmt.Sprintf("%d-non-existent-nex-resource-dir", _fixtures.seededRand.Int()))
							nodeConfig.WorkloadTypes = []controlapi.NexWorkload{controlapi.NexWorkloadNative, controlapi.NexWorkloadV8, controlapi.NexWorkloadWasm}
						})

						It("should not return an error", func(ctx SpecContext) {
							Expect(nexnode.CmdPreflight(opts, nodeOpts, ctxx, cancel, log)).To(BeNil())
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

						It("should not fetch the default vmlinux kernel", func(ctx SpecContext) {
							_, err := os.Stat(filepath.Join(nodeConfig.DefaultResourceDir, "vmlinux"))
							Expect(err).NotTo(BeNil())
						})

						It("should not fetch the default agent rootfs template", func(ctx SpecContext) {
							_, err := os.Stat(filepath.Join(nodeConfig.DefaultResourceDir, "rootfs.ext4"))
							Expect(err).NotTo(BeNil())
						})
					})

					Context("when the node is configured with sandboxing enabled", func() {
						BeforeEach(func() {
							nodeConfig.NoSandbox = false
						})

						It("should return an error", func(ctx SpecContext) {
							err := nexnode.CmdPreflight(opts, nodeOpts, ctxx, cancel, log)
							Expect(err).ToNot(BeNil())
							Expect(err.Error()).To(ContainSubstring("windows host must be configured to run in no sandbox mode"))
						})
					})
				})
			})
		})
	})

	DescribeTableSubtree("up", func(sandbox bool) {
		Context("when the specified configuration file does not exist", func() {
			BeforeEach(func() {
				nodeOpts.ConfigFilepath = filepath.Join(os.TempDir(), fmt.Sprintf("%d-non-existent-nex-conf.json", _fixtures.seededRand.Int()))
				nodeConfig.NoSandbox = !sandbox
			})

			It("should return an error", func(ctx SpecContext) {
				err := nexnode.CmdUp(opts, nodeOpts, ctxx, cancel, nodeKey, log)
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("open %s: The system cannot find the file specified", nodeOpts.ConfigFilepath)))
			})
		})

		Context("when the specified node configuration file exists", func() {
			BeforeEach(func() {
				nodeConfig = models.DefaultNodeConfiguration()
				nodeOpts.ConfigFilepath = path.Join(os.TempDir(), fmt.Sprintf("%d-spec-nex-conf.json", _fixtures.seededRand.Int()))
				nodeConfig.WorkloadTypes = []controlapi.NexWorkload{controlapi.NexWorkloadNative, controlapi.NexWorkloadV8, controlapi.NexWorkloadWasm}

				nodeConfig.NoSandbox = !sandbox
				nodeKey, _ = nkeys.CreateServer()
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

					It("should not return an error", func(ctx SpecContext) {
						err := nexnode.CmdUp(opts, nodeOpts, ctxx, cancel, nodeKey, log)
						Expect(err).To(BeNil())
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

						nodeConfig.MachinePoolSize = 1
					})

					AfterEach(func() {
						p, _ := os.FindProcess(os.Getpid())
						_ = p.Signal(os.Interrupt)

						node = nil
						nodeID = nil
						nodeProxy = nil
						nodeKey = nil
					})

					JustBeforeEach(func() {
						var err error

						_ = nexnode.CmdPreflight(opts, nodeOpts, ctxx, cancel, log)
						node, err = nexnode.NewNode(nodeKey, opts, nodeOpts, ctxx, cancel, log)
						Expect(err).To(BeNil())

						go node.Start()

						nodeID, _ = node.PublicKey()
						nodeProxy = nexnode.NewNodeProxyWith(node)
						time.Sleep(time.Millisecond * 1000)
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

					It("should initialize a machine manager to manage firecracker VMs and communicate with running agents", func(ctx SpecContext) {
						Expect(nodeProxy.WorkloadManager()).ToNot(BeNil())
					})

					It("should initialize an API listener", func(ctx SpecContext) {
						Expect(nodeProxy.APIListener()).ToNot(BeNil())
					})

					It("should initialize a telemetry instance", func(ctx SpecContext) {
						Expect(nodeProxy.Telemetry()).ToNot(BeNil())
					})

					Describe("machine manager", func() {
						var manager *nexnode.WorkloadManager
						var managerProxy *nexnode.WorkloadManagerProxy

						AfterEach(func() {
							manager = nil
							managerProxy = nil
						})

						JustBeforeEach(func() {
							manager = nodeProxy.WorkloadManager()
							managerProxy = nexnode.NewWorkloadManagerProxyWith(manager)

							time.Sleep(time.Millisecond * 1000) // allow enough time for the pool to warm up...
						})

						It("should use the provided logger instance", func(ctx SpecContext) {
							Expect(managerProxy.Log()).To(Equal(log))
						})

						It("should receive a reference to the node configuration", func(ctx SpecContext) {
							Expect(managerProxy.NodeConfiguration()).To(Equal(nodeProxy.NodeConfiguration())) // FIXME-- assert that it is === to the current nex node config JSON
						})

						It("should initialize an internal NATS server for private communication between running VMs and the host", func(ctx SpecContext) {
							Expect(managerProxy.InternalNATS()).ToNot(BeNil())
						})

						It("should initialize a connection to the internal NATS server", func(ctx SpecContext) {
							Expect(managerProxy.InternalNATSConn()).ToNot(BeNil())
						})

						It("should receive a reference to the telemetry instance", func(ctx SpecContext) {
							Expect(managerProxy.Telemetry()).To(Equal(nodeProxy.Telemetry()))
						})

						Describe("VM pool", func() {
							Context("when no workloads have been deployed", func() {
								It("should complete an agent handshake for each VM in the configured pool size", func(ctx SpecContext) {
									workloads, _ := nodeProxy.WorkloadManager().RunningWorkloads()
									for _, workload := range workloads {
										subsz, _ := managerProxy.InternalNATS().Subsz(&server.SubszOptions{
											Subscriptions: true,
											Test:          fmt.Sprintf("agentint.%s.handshake", workload.Id),
										})
										Expect(subsz.Subs[0].Msgs).To(Equal(1))
									}
								})
							})

							Describe("deploying a native binary workload", func() {
								var deployRequest *controlapi.DeployRequest
								var err error

								AfterEach(func() {
									os.Remove("./echoservice.exe")
								})

								JustBeforeEach(func() {
									deployRequest, err = newDeployRequest(*nodeID, "echoservice", "nex example echoservice", "./echoservice.exe", map[string]string{"NATS_URL": "nats://127.0.0.1:4222"}, []string{}, log)
									Expect(err).To(BeNil())

									nodeClient := controlapi.NewApiClientWithNamespace(_fixtures.natsConn, time.Millisecond*1000, "default", log)
									_, err = nodeClient.StartWorkload(deployRequest)

									time.Sleep(time.Millisecond * 1000)
								})

								Context("when the native binary is not statically-linked", func() {
									BeforeEach(func() {
										cmd := exec.Command("go", "build", "../examples/echoservice")
										_ = cmd.Start()
										_ = cmd.Wait()
									})

									It("should deploy the native workload", func(ctx SpecContext) {
										Expect(err).To(BeNil())
									})
								})

								Context("when the native binary is statically-linked", func() {
									BeforeEach(func() {
										cmd := exec.Command("go", "build", "-tags", "netgo", "-ldflags", "-extldflags -static", "../examples/echoservice")
										_ = cmd.Start()
										_ = cmd.Wait()

										time.Sleep(time.Millisecond * 1000)
									})

									It("should deploy the native workload", func(ctx SpecContext) {
										Expect(err).To(BeNil())
									})
								})
							})

							Describe("deploying a v8 workload", func() {
								var deployRequest *controlapi.DeployRequest
								var triggerSubject string
								var err error

								Describe("echoservice", func() {
									Context("when the javascript is valid", func() {
										JustBeforeEach(func() {
											triggerSubject = "helloworld"
											deployRequest, err = newDeployRequest(*nodeID, "echofunction", "nex example echoservice", "../examples/v8/echofunction/src/echofunction.js", map[string]string{}, []string{triggerSubject}, log)
											Expect(err).To(BeNil())

											nodeClient := controlapi.NewApiClientWithNamespace(_fixtures.natsConn, time.Millisecond*1000, "default", log)
											_, err = nodeClient.StartWorkload(deployRequest)

											time.Sleep(time.Millisecond * 1000)
										})

										It("should not deploy the v8 workload", func(ctx SpecContext) {
											Expect(err.Error()).To(ContainSubstring("V8 is not supported on this platform"))
										})
									})
								})

							})

							Describe("deploying a wasm workload", func() {
								var deployRequest *controlapi.DeployRequest
								var triggerSubject string
								var err error

								JustBeforeEach(func() {
									triggerSubject = "helloworld"
									deployRequest, err = newDeployRequest(*nodeID, "echofunction", "nex example echoservice", "../examples/wasm/echofunction/echofunction.wasm", map[string]string{}, []string{triggerSubject}, log)
									Expect(err).To(BeNil())

									nodeClient := controlapi.NewApiClientWithNamespace(_fixtures.natsConn, time.Millisecond*1000, "default", log)
									_, err = nodeClient.StartWorkload(deployRequest)

									time.Sleep(time.Millisecond * 1000)
								})

								Context("when the wasm is valid", func() {
									It("should deploy the wasm workload", func(ctx SpecContext) {
										Expect(err).To(BeNil())
									})
								})
							})
						})
					})
				})
			})
		})
	},
		Entry("no-sandbox", false), // no-sandbox mode
	)
})

func cacheWorkloadArtifact(nc *nats.Conn, filename string) (string, string, controlapi.NexWorkload, error) {
	js, err := nc.JetStream()
	if err != nil {
		panic(err)
	}
	var bucket nats.ObjectStore
	bucket, err = js.ObjectStore("NEXCLIFILES")
	if err != nil {
		bucket, err = js.CreateObjectStore(&nats.ObjectStoreConfig{
			Bucket:      agentapi.WorkloadCacheBucket,
			Description: "Ad hoc object storage for NEX CLI developer mode uploads",
		})
		if err != nil {
			return "", "", "", err
		}
	}
	bytes, err := os.ReadFile(filename)
	if err != nil {
		return "", "", "", err
	}
	key := filepath.Base(filename)
	key = strings.ReplaceAll(key, ".", "")

	_, err = bucket.PutBytes(key, bytes)
	if err != nil {
		return "", "", "", err
	}

	var workloadType controlapi.NexWorkload
	switch strings.Replace(filepath.Ext(filename), ".", "", 1) {
	case "exe":
		workloadType = controlapi.NexWorkloadNative
	case "js":
		workloadType = controlapi.NexWorkloadV8
	case "wasm":
		workloadType = controlapi.NexWorkloadWasm
	default:
		workloadType = controlapi.NexWorkloadNative
	}

	return fmt.Sprintf("nats://%s/%s", "NEXCLIFILES", key), key, workloadType, nil
}

func resolveNodeTargetPublicXKey(nodeID string, log *slog.Logger) (*string, error) {
	nodeClient := controlapi.NewApiClientWithNamespace(_fixtures.natsConn, time.Millisecond*250, "default", log)

	var info *controlapi.InfoResponse
	for info == nil {
		info, _ = nodeClient.NodeInfo(nodeID)
		time.Sleep(time.Millisecond * 25)
	}

	return &info.PublicXKey, nil
}

// newDeployRequest() generates a new deploy request given the workload name, description, and file path
func newDeployRequest(nodeID, name, desc, path string, env map[string]string, triggerSubjects []string, log *slog.Logger) (*controlapi.DeployRequest, error) { // initializes new sender and issuer keypairs and returns a new deploy request
	senderKey, _ := nkeys.CreateCurveKeys()
	issuerKey, _ := nkeys.CreateAccount()

	location, _, workloadType, err := cacheWorkloadArtifact(_fixtures.natsConn, path)
	if err != nil {
		return nil, err
	}

	targetPublicXKey, err := resolveNodeTargetPublicXKey(nodeID, log)
	if err != nil {
		return nil, err
	}

	opts := []controlapi.RequestOption{
		controlapi.WorkloadName(name),
		controlapi.WorkloadType(workloadType),
		controlapi.WorkloadDescription(desc),
		controlapi.Location(location),
		// controlapi.Checksum(""),
		controlapi.SenderXKey(senderKey),
		controlapi.Issuer(issuerKey),
		controlapi.TargetNode(nodeID),
		controlapi.TargetPublicXKey(*targetPublicXKey),
		controlapi.TriggerSubjects(triggerSubjects),
	}

	for k, v := range env {
		opts = append(opts, controlapi.EnvironmentValue(k, v))
	}

	return controlapi.NewDeployRequest(opts...)
}
