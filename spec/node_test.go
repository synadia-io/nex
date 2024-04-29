//go:build linux

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
	"net"
	"net/http"
	"net/http/httptest"
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
	"github.com/rs/xid"

	shandler "github.com/jordan-rash/slog-handler"
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
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
			Servers:      _fixtures.natsServer.ClientURL(),
			SkipContexts: true,
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

			agentPath, err := filepath.Abs("../agent/cmd/nex-agent")
			Expect(err).To(BeNil())

			os.Setenv("PATH", fmt.Sprintf("%s:%s", os.Getenv("PATH"), agentPath))

			snapshotAgentRootFSPath = filepath.Join(os.TempDir(), fmt.Sprintf("%d-rootfs.ext4", _fixtures.seededRand.Int()))
			cmd := exec.Command("sudo", "go", "run", "../nex", "fs", "--agent", "../agent/cmd/nex-agent/nex-agent")
			_, err = cmd.CombinedOutput()
			Expect(err).To(BeNil())
			Expect(cmd.ProcessState.ExitCode()).To(BeZero())

			compressedRootFS, err := os.Open("./rootfs.ext4.gz")
			Expect(err).To(BeNil())

			uncompressedRootFS, err := gzip.NewReader(bufio.NewReader(compressedRootFS))
			Expect(err).To(BeNil())

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
				nodeConfig = models.DefaultNodeConfiguration()
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

	DescribeTableSubtree("up", func(sandbox bool) {
		Context("when the specified configuration file does not exist", func() {
			BeforeEach(func() {
				nodeOpts.ConfigFilepath = filepath.Join(os.TempDir(), fmt.Sprintf("%d-non-existent-nex-conf.json", _fixtures.seededRand.Int()))
				nodeConfig.NoSandbox = !sandbox
			})

			It("should return an error", func(ctx SpecContext) {
				err := nexnode.CmdUp(opts, nodeOpts, ctxx, cancel, log)
				Expect(err).ToNot(BeNil())
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("open %s: no such file or directory", nodeOpts.ConfigFilepath)))
			})
		})

		Context("when the specified node configuration file exists", func() {
			BeforeEach(func() {
				nodeConfig = models.DefaultNodeConfiguration()
				nodeOpts.ConfigFilepath = path.Join(os.TempDir(), fmt.Sprintf("%d-spec-nex-conf.json", _fixtures.seededRand.Int()))

				nodeConfig.NoSandbox = !sandbox
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
						Expect(err.Error()).To(ContainSubstring("failed to initialize node"))
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
						node.Stop()

						node = nil
						nodeID = nil
						nodeProxy = nil
					})

					JustBeforeEach(func() {
						var err error

						_ = nexnode.CmdPreflight(opts, nodeOpts, ctxx, cancel, log)
						node, err = nexnode.NewNode(opts, nodeOpts, ctxx, cancel, log)
						Expect(err).To(BeNil())

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

					It("should initialize a connection to the internal NATS server", func(ctx SpecContext) {
						Expect(nodeProxy.InternalNATSConn()).ToNot(BeNil())
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

					// Context("when node options enable otel", func() {
					// TODO
					// })

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

						It("should receive a reference to the internal NATS server connection", func(ctx SpecContext) {
							Expect(managerProxy.InternalNATSConn()).To(Equal(nodeProxy.InternalNATSConn()))
						})

						It("should receive a reference to the telemetry instance", func(ctx SpecContext) {
							Expect(managerProxy.Telemetry()).To(Equal(nodeProxy.Telemetry()))
						})

						Describe("VM pool", func() {
							Context("when no workloads have been deployed", func() {
								It("should complete an agent handshake for each VM in the configured pool size", func(ctx SpecContext) {
									workloads, _ := nodeProxy.WorkloadManager().RunningWorkloads()
									for _, workload := range workloads {
										subsz, _ := nodeProxy.InternalNATS().Subsz(&server.SubszOptions{
											Subscriptions: true,
											Test:          fmt.Sprintf("agentint.%s.handshake", workload.Id),
										})
										Expect(subsz.Subs[0].Msgs).To(Equal(1))
									}
								})

								// It("should keep a reference to all running agent processes", func(ctx SpecContext) {
								// 	Expect(len(managerProxy.AllAgents())).To(Equal(nodeProxy.NodeConfiguration().MachinePoolSize))
								// })

								// It("should maintain the configured number of warm agents in the pool", func(ctx SpecContext) {
								// 	Expect(len(managerProxy.PoolAgents())).To(Equal(nodeProxy.NodeConfiguration().MachinePoolSize))
								// })

								// FIXME-- redesign this test when we refactor to test both Firecracker and no-sandbox agents...
								// It("should assign 192.168.127.2 to the first warm agent in the pool", func(ctx SpecContext) {
								// 	var workloadID string
								// 	for k := range managerProxy.PoolAgents() {
								// 		workloadID = k
								// 		break
								// 	}

								// 	agent := nexnode.NewAgentProxyWith(managerProxy.PoolAgents()[workloadID])
								// 	Expect(agent.IP().String()).To(Equal("192.168.127.2"))
								// })
							})

							Describe("deploying an ELF binary workload", func() {
								var deployRequest *controlapi.DeployRequest
								var err error

								AfterEach(func() {
									os.Remove("./echoservice")
								})

								JustBeforeEach(func() {
									deployRequest, err = newDeployRequest(*nodeID, "echoservice", "nex example echoservice", "./echoservice", map[string]string{"NATS_URL": "nats://127.0.0.1:4222"}, []string{}, log)
									Expect(err).To(BeNil())

									nodeClient := controlapi.NewApiClientWithNamespace(_fixtures.natsConn, time.Millisecond*1000, "default", log)
									_, err = nodeClient.StartWorkload(deployRequest)

									time.Sleep(time.Millisecond * 1000)
								})

								Context("when the ELF binary is not statically-linked", func() {
									BeforeEach(func() {
										cmd := exec.Command("go", "build", "../examples/echoservice")
										_ = cmd.Start()
										_ = cmd.Wait()
									})

									It("should [fail to] deploy the ELF workload", func(ctx SpecContext) {
										if sandbox {
											Expect(err.Error()).To(ContainSubstring("elf binary contains at least one dynamically linked dependency"))
										} else {
											Expect(err).To(BeNil())
										}
									})

									// It("should keep a reference to all running agent processes", func(ctx SpecContext) {
									// 	Expect(len(managerProxy.AllAgents())).To(Equal(1))
									// })

									// It("should maintain the configured number of warm agents in the pool", func(ctx SpecContext) {
									// 	Expect(len(managerProxy.PoolAgents())).To(Equal(nodeProxy.NodeConfiguration().MachinePoolSize))
									// })
								})

								Context("when the ELF binary is statically-linked", func() {
									BeforeEach(func() {
										cmd := exec.Command("go", "build", "-tags", "netgo", "-ldflags", "-extldflags -static", "../examples/echoservice")
										_ = cmd.Start()
										_ = cmd.Wait()

										time.Sleep(time.Millisecond * 1000)
									})

									It("should deploy the ELF workload", func(ctx SpecContext) {
										Expect(err).To(BeNil())
									})

									// It("should keep a reference to all running agent processes", func(ctx SpecContext) {
									// 	Expect(len(managerProxy.AllAgents())).To(Equal(2))
									// })

									// It("should maintain the configured number of warm agents in the pool", func(ctx SpecContext) {
									// 	Expect(len(managerProxy.PoolAgents())).To(Equal(nodeProxy.NodeConfiguration().MachinePoolSize))
									// })
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
											Expect(err).To(BeNil())

											time.Sleep(time.Millisecond * 1000)
										})

										It("should deploy the v8 workload", func(ctx SpecContext) {
											Expect(err).To(BeNil())
										})

										// It("should keep a reference to all running agent processes", func(ctx SpecContext) {
										// 	Expect(len(managerProxy.AllAgents())).To(Equal(2))
										// })

										// It("should maintain the configured number of warm agents in the pool", func(ctx SpecContext) {
										// 	Expect(len(managerProxy.PoolAgents())).To(Equal(nodeProxy.NodeConfiguration().MachinePoolSize))
										// })

										Describe("triggering the deployed function", func() {
											var respmsg *nats.Msg

											JustBeforeEach(func() {
												respmsg, err = _fixtures.natsConn.Request(triggerSubject, []byte("hello world"), time.Millisecond*5000)
												Expect(err).To(BeNil())
											})

											It("should respond to the request by echoing the payload", func(ctx SpecContext) {
												Expect(respmsg).NotTo(BeNil())
												Expect(respmsg.Data).To(Equal([]byte(fmt.Sprintf("{\"triggered_on\":\"%s\",\"payload\":\"hello world\"}", triggerSubject))))
											})
										})
									})
								})

								Describe("host services", func() {
									Context("key value service", func() {
										Context("when the javascript is valid", func() {
											JustBeforeEach(func() {
												triggerSubject = "hellokvservice"
												deployRequest, err = newDeployRequest(*nodeID, "kvhostservice", "nex key value service example", "../examples/v8/echofunction/src/kv.js", map[string]string{}, []string{triggerSubject}, log)
												Expect(err).To(BeNil())

												nodeClient := controlapi.NewApiClientWithNamespace(_fixtures.natsConn, time.Millisecond*1000, "default", log)
												_, err = nodeClient.StartWorkload(deployRequest)
												Expect(err).To(BeNil())

												time.Sleep(time.Millisecond * 1000)
											})

											Describe("triggering the deployed function", func() {
												var respmsg *nats.Msg

												JustBeforeEach(func() {
													respmsg, err = _fixtures.natsConn.Request(triggerSubject, []byte("hello!"), time.Millisecond*15000)
													Expect(err).To(BeNil())
												})

												It("should respond to the request with the list of keys and value of hello2", func(ctx SpecContext) {
													Expect(respmsg).NotTo(BeNil())

													type hostServicesExampleResp struct {
														Keys   []string `json:"keys"`
														Hello2 string   `json:"hello2"`
													}

													var resp *hostServicesExampleResp
													err = json.Unmarshal(respmsg.Data, &resp)
													Expect(err).To(BeNil())

													Expect(resp).ToNot(BeNil())

													Expect(len(resp.Keys)).To(Equal(1))
													Expect(resp.Keys[0]).To(Equal("hello2"))
													Expect(resp.Hello2).To(Equal("hello!"))
												})
											})
										})
									})

									Context("messaging service", func() {
										Context("when the javascript is valid", func() {
											JustBeforeEach(func() {
												triggerSubject = "hellomessagingservice"
												deployRequest, err = newDeployRequest(*nodeID, "messaginghostservice", "nex messaging service example", "../examples/v8/echofunction/src/messaging.js", map[string]string{}, []string{triggerSubject}, log)
												Expect(err).To(BeNil())

												nodeClient := controlapi.NewApiClientWithNamespace(_fixtures.natsConn, time.Millisecond*1000, "default", log)
												_, err = nodeClient.StartWorkload(deployRequest)
												Expect(err).To(BeNil())

												time.Sleep(time.Millisecond * 1000)
											})

											Describe("triggering the deployed function", func() {
												var respmsg *nats.Msg
												var sub *nats.Subscription

												BeforeEach(func() {
													sub, _ = _fixtures.natsConn.Subscribe("hello.world.request", func(msg *nats.Msg) {
														resp := fmt.Sprintf("resp: %s", msg.Data)
														_ = msg.Respond([]byte(resp))
													})

													sub, _ = _fixtures.natsConn.Subscribe("hello.world.request.many", func(msg *nats.Msg) {
														resp := fmt.Sprintf("resp #1: %s", msg.Data)
														_ = msg.Respond([]byte(resp))

														resp = fmt.Sprintf("resp #2: %s", msg.Data)
														_ = msg.Respond([]byte(resp))
													})
												})

												AfterEach(func() {
													_ = sub.Unsubscribe()
													sub = nil
												})

												JustBeforeEach(func() {
													respmsg, err = _fixtures.natsConn.Request(triggerSubject, []byte("asdfghjkl;'"), time.Millisecond*7500)
													Expect(err).To(BeNil())
												})

												It("should respond to the request with the hello.world.request response value", func(ctx SpecContext) {
													Expect(respmsg).NotTo(BeNil())

													type hostServicesExampleResp struct {
														HelloWorldRequest     string   `json:"hello.world.request"`
														HelloWorldRequestMany []string `json:"hello.world.request.many"`
													}

													var resp *hostServicesExampleResp
													err = json.Unmarshal(respmsg.Data, &resp)
													Expect(err).To(BeNil())

													Expect(resp).ToNot(BeNil())

													Expect(resp.HelloWorldRequest).ToNot(BeNil())
													Expect(resp.HelloWorldRequest).To(Equal("resp: asdfghjkl;'"))
												})

												It("should respond to the request with the hello.world.request.many response values", func(ctx SpecContext) {
													Expect(respmsg).NotTo(BeNil())

													type hostServicesExampleResp struct {
														HelloWorldRequest     string   `json:"hello.world.request"`
														HelloWorldRequestMany []string `json:"hello.world.request.many"`
													}

													var resp *hostServicesExampleResp
													err = json.Unmarshal(respmsg.Data, &resp)
													Expect(err).To(BeNil())

													Expect(resp).ToNot(BeNil())

													Expect(resp.HelloWorldRequestMany).ToNot(BeNil())
													Expect(len(resp.HelloWorldRequestMany)).To(Equal(2))
													Expect(resp.HelloWorldRequestMany[0]).To(Equal("resp #1: asdfghjkl;'"))
													Expect(resp.HelloWorldRequestMany[1]).To(Equal("resp #2: asdfghjkl;'"))
												})
											})
										})
									})

									Context("http service", func() {
										var ts *httptest.Server

										Context("when the javascript is valid", func() {
											JustBeforeEach(func() {
												ts.Start()

												exbytes, err := os.ReadFile("../examples/v8/echofunction/src/http.js")
												Expect(err).To(BeNil())

												var jsexample string

												if sandbox {
													jsexample = strings.ReplaceAll(string(exbytes), "https://example.org", fmt.Sprintf("http://192.168.127.1:%d", ts.Listener.Addr().(*net.TCPAddr).Port))
												} else {
													jsexample = strings.ReplaceAll(string(exbytes), "https://example.org", fmt.Sprintf("http://127.0.0.1:%d", ts.Listener.Addr().(*net.TCPAddr).Port))
												}

												filename := xid.New()
												tmpfilePath := path.Join(os.TempDir(), fmt.Sprintf("http-hostservice-example-%s.js", filename.String()))
												err = os.WriteFile(tmpfilePath, []byte(jsexample), 0644)
												Expect(err).To(BeNil())

												triggerSubject = "hellohttpservice"
												deployRequest, err = newDeployRequest(*nodeID, "httphostservice", "nex http service example", tmpfilePath, map[string]string{}, []string{triggerSubject}, log)
												Expect(err).To(BeNil())

												nodeClient := controlapi.NewApiClientWithNamespace(_fixtures.natsConn, time.Millisecond*3000, "default", log)
												_, err = nodeClient.StartWorkload(deployRequest)
												Expect(err).To(BeNil())

												os.Remove(tmpfilePath)
												time.Sleep(time.Millisecond * 1000)
											})

											Describe("triggering the deployed function", func() {
												type hostServicesExampleHttpResp struct {
													Status  int             `json:"status"`
													Headers json.RawMessage `json:"headers"`
													Body    *string         `json:"body"`
												}

												type hostServicesExampleResp struct {
													Get    *hostServicesExampleHttpResp `json:"get"`
													Post   *hostServicesExampleHttpResp `json:"post"`
													Put    *hostServicesExampleHttpResp `json:"put"`
													Patch  *hostServicesExampleHttpResp `json:"patch"`
													Delete *hostServicesExampleHttpResp `json:"delete"`
													Head   *hostServicesExampleHttpResp `json:"head"`
												}

												var respmsg *nats.Msg
												var resp *hostServicesExampleResp

												BeforeEach(func() {
													ts = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
														switch r.Method {
														case http.MethodGet:
															w.WriteHeader(200)
														case http.MethodPost:
															w.WriteHeader(201)
														case http.MethodPut:
															w.WriteHeader(204)
														case http.MethodPatch:
															w.WriteHeader(204)
														case http.MethodDelete:
															w.WriteHeader(204)
														case http.MethodHead:
															w.WriteHeader(200)
														default:
															// no-op
														}
													}))

													ts.EnableHTTP2 = true
													ts.Listener.Close()

													listener, err := net.Listen("tcp", ":0")
													Expect(err).To(BeNil())

													ts.Listener = listener
												})

												AfterEach(func() {
													ts.Close()
													ts = nil

													respmsg = nil
												})

												JustBeforeEach(func() {
													respmsg, err = _fixtures.natsConn.Request(triggerSubject, []byte("payload"), time.Millisecond*5000)
													Expect(err).To(BeNil())

													err = json.Unmarshal(respmsg.Data, &resp)
													Expect(err).To(BeNil())
													Expect(resp).ToNot(BeNil())
												})

												It("should send a GET request to the configured endpoint", func(ctx SpecContext) {
													Expect(resp.Get).NotTo(BeNil())
													Expect(resp.Get.Status).To(Equal(200))
												})

												It("should send a POST request to the configured endpoint", func(ctx SpecContext) {
													Expect(resp.Post).NotTo(BeNil())
													Expect(resp.Post.Status).To(Equal(201))
												})

												It("should send a PUT request to the configured endpoint", func(ctx SpecContext) {
													Expect(resp.Put).NotTo(BeNil())
													Expect(resp.Put.Status).To(Equal(204))
												})

												It("should send a PATCH request to the configured endpoint", func(ctx SpecContext) {
													Expect(resp.Patch).NotTo(BeNil())
													Expect(resp.Patch.Status).To(Equal(204))
												})

												It("should send a DELETE request to the configured endpoint", func(ctx SpecContext) {
													Expect(resp.Delete).NotTo(BeNil())
													Expect(resp.Delete.Status).To(Equal(204))
												})

												It("should send a HEAD request to the configured endpoint", func(ctx SpecContext) {
													Expect(resp.Head).NotTo(BeNil())
													Expect(resp.Head.Status).To(Equal(200))
												})
											})
										})
									})

									Context("object store service", func() {
										Context("when the javascript is valid", func() {
											JustBeforeEach(func() {
												triggerSubject = "helloobjectstoreservice"
												deployRequest, err = newDeployRequest(*nodeID, "objectstorehostservice", "nex object store service example", "../examples/v8/echofunction/src/objectstore.js", map[string]string{}, []string{triggerSubject}, log)
												Expect(err).To(BeNil())

												nodeClient := controlapi.NewApiClientWithNamespace(_fixtures.natsConn, time.Millisecond*1000, "default", log)
												_, err = nodeClient.StartWorkload(deployRequest)
												Expect(err).To(BeNil())

												time.Sleep(time.Millisecond * 1000)
											})

											Describe("triggering the deployed function", func() {
												var respmsg *nats.Msg

												JustBeforeEach(func() {
													respmsg, err = _fixtures.natsConn.Request(triggerSubject, []byte("hello!"), time.Millisecond*15000)
													Expect(err).To(BeNil())
												})

												It("should respond to the request with the list of keys and value of hello2", func(ctx SpecContext) {
													Expect(respmsg).NotTo(BeNil())

													type hostServicesExampleResp struct {
														Hello2 string             `json:"hello2"`
														List   []*nats.ObjectInfo `json:"list"`
													}

													var resp *hostServicesExampleResp
													err = json.Unmarshal(respmsg.Data, &resp)
													Expect(err).To(BeNil())

													Expect(resp).ToNot(BeNil())

													Expect(len(resp.List)).To(Equal(1))
													Expect(resp.List[0].Name).To(Equal("hello2"))
													Expect(resp.Hello2).To(Equal("hello!"))
												})
											})
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

									// It("should keep a reference to all running agent processes", func(ctx SpecContext) {
									// 	Expect(len(managerProxy.AllAgents())).To(Equal(2))
									// })

									// It("should maintain the configured number of warm agents in the pool", func(ctx SpecContext) {
									// 	Expect(len(managerProxy.PoolAgents())).To(Equal(nodeProxy.NodeConfiguration().MachinePoolSize))
									// })
								})
							})
						})
					})
				})
			})
		})
	},
		Entry("sandbox", true),     // sandbox mode
		Entry("no-sandbox", false), // no-sandbox mode
	)
})

func cacheWorkloadArtifact(nc *nats.Conn, filename string) (string, string, string, error) {
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

	var workloadType string
	switch strings.Replace(filepath.Ext(filename), ".", "", 1) {
	case "js":
		workloadType = agentapi.NexExecutionProviderV8
	case "wasm":
		workloadType = agentapi.NexExecutionProviderWasm
	default:
		workloadType = "elf"
	}

	return fmt.Sprintf("nats://%s/%s", "NEXCLIFILES", key), key, workloadType, nil
}

func resolveNodeTargetPublicXKey(nodeID string, log *slog.Logger) (*string, error) {
	nodeClient := controlapi.NewApiClientWithNamespace(_fixtures.natsConn, time.Millisecond*250, "default", log)

	nodes, err := nodeClient.ListAllNodes()
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		log.Error("no nodes discovered", slog.Any("nc_server", _fixtures.natsConn.Servers()))
		return nil, errors.New("no nodes discovered")
	}

	for _, candidate := range nodes {
		if strings.EqualFold(candidate.NodeId, nodeID) {
			info, err := nodeClient.NodeInfo(nodes[0].NodeId)
			if err != nil {
				return nil, fmt.Errorf("failed to get node info for potential execution target: %s", err)
			}

			return &info.PublicXKey, nil
		}
	}

	return nil, fmt.Errorf("no node discovered which matched %s", nodeID)
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
