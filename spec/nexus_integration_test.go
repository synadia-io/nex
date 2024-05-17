package spec

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	shandler "github.com/jordan-rash/slog-handler"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"github.com/synadia-io/nex/internal/models"
)

const (
	defaultFileMode     = os.FileMode(int(0770)) // owner and group r/w/x
	defaultWorkloadType = agentapi.NexExecutionProviderELF
	fileExtensionJS     = "js"
	fileExtensionWasm   = "wasm"

	objectStoreName     = "NEXCLIFILES"
	objectStoreMaxBytes = uint(100 * 1024 * 1024) // 100 MB
)

var _ = FDescribe("nexus integration", func() {
	var nc *nats.Conn
	var logger *slog.Logger
	var releaseOnce sync.Once

	BeforeEach(func() {
		releaseOnce.Do(func() {
			fmt.Printf("running goreleaser")

			cmd := exec.Command("goreleaser", "build", "--snapshot", "--clean", "--config", "./nexus_integration_builds.yml")
			output, err := cmd.CombinedOutput()
			Expect(err).To(BeNil())

			fmt.Printf(string(output))

			logger = slog.New(shandler.NewHandler(shandler.WithLogLevel(slog.LevelDebug), shandler.WithColor()))

			nc, err = models.GenerateConnectionFromOpts(&models.Options{
				Servers: os.Getenv("NEX_CLUSTER_URL"),
				Creds:   os.Getenv("NEX_CLUSTER_CREDS"),
			}, logger)
			Expect(err).To(BeNil())
		})
	})

	// FIXME-- cleanup the connection
	// AfterAll(func() {
	// 	os.Remove("./dist")
	// 	nc.Drain()
	// })

	DescribeTableSubtree("nex run", func(arch, os, namespace string, attempts int, timeout time.Duration) {
		Context("when attempting to run a valid binary", func() {
			It("should deploy the workload successfully", func(ctx SpecContext) {
				path := fmt.Sprintf("./dist/echoservice_%s_%s/echoservice", os, arch)
				if strings.EqualFold(arch, "amd64") { // FIXME use string builder instead
					path = fmt.Sprintf("./dist/echoservice_%s_%s_v1/echoservice", os, arch) // amd64 dist directory is amd64_v1
				}
				if strings.EqualFold(os, "windows") {
					path = fmt.Sprintf("%s.exe", path)
				}

				var err error

				i := 1
				for i <= attempts {
					fmt.Printf("attempting nex run %v of %v on %s/%s\n", i, attempts, arch, os)

					_err := devrun(nc, path, namespace, logger, timeout, true, []string{}) // FIXME-- add as subtree params
					if _err != nil {
						errmsg := fmt.Sprintf("❌ nex run %v of %v failed; %s\n", i, attempts, _err.Error())
						err = errors.Join(err, errors.New(errmsg))
						fmt.Printf(errmsg)
					} else {
						fmt.Printf("completed nex run %v of %v on %s/%s\n", i, attempts, arch, os)
					}

					i++
				}

				Expect(err).To(BeNil())
			})
		})
	},
		Entry("amd64 linux", "amd64", "linux", "default", 10, 2500*time.Millisecond),
		Entry("arm64 linux", "arm64", "linux", "default", 10, 2500*time.Millisecond),

		Entry("amd64 windows", "amd64", "windows", "default", 10, 2500*time.Millisecond),
	)
})

// FIXME-- drop devrun and just use the run command
func devrun(nc *nats.Conn, path, namespace string, logger *slog.Logger, timeout time.Duration, stopIfExists bool, triggerSubjects []string) error {
	// developer mode can have a smaller discovery timeout, since we're assuming there's a NEX
	// node "nearby"
	nodeClient := controlapi.NewApiClientWithNamespace(nc, timeout, namespace, logger)

	candidates, err := nodeClient.ListAllNodes()
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return errors.New("unable to locate candidate node - no nodes discovered")
	}
	target := candidates[0]
	info, err := nodeClient.NodeInfo(target.NodeId)
	if err != nil {
		return fmt.Errorf("failed to get node info for potential execution target: %s", err)
	}

	issuerKp, err := readOrGenerateIssuer()
	if err != nil {
		return err
	}
	publisherXKey, err := readOrGeneratePublisher()
	if err != nil {
		return err
	}

	targetPublicXkey := info.PublicXKey
	workloadUrl, workloadName, workloadType, err := uploadWorkload(nc, models.DevRunOptions{
		Filename: path,
		AutoStop: stopIfExists,
		// DevBucketMaxBytes: ,
	})
	if err != nil {
		return err
	}

	if workloadType == "v8" && len(triggerSubjects) == 0 {
		return errors.New("cannot start a function-type workload without specifying at least one trigger subject")
	}

	if stopIfExists {
		for _, machine := range info.Machines {
			if machine.Workload.Name == workloadName {
				fmt.Printf("Workload %s (%s) already exists on the target. Attempting to stop it\n", workloadName, machine.Id)
				stopRequest, err := controlapi.NewStopRequest(machine.Id, workloadName, target.NodeId, issuerKp)
				if err != nil {
					return err
				}
				stopResp, err := nodeClient.StopWorkload(stopRequest)
				if err != nil {
					return err
				}
				if !stopResp.Stopped {
					return errors.New("target node failed to stop the existing workload. This may result in inconsistency or unexpected scale-out")
				}
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

	request, err := controlapi.NewDeployRequest(
		controlapi.Location(workloadUrl),
		// TODO controlapi.Environment(),
		// TODO controlapi.Essential(essential),
		controlapi.Issuer(issuerKp),
		controlapi.SenderXKey(publisherXKey),
		controlapi.TargetNode(target.NodeId),
		controlapi.TargetPublicXKey(targetPublicXkey),
		controlapi.TriggerSubjects(triggerSubjects),
		controlapi.WorkloadName(workloadName),
		controlapi.WorkloadType(workloadType),
		controlapi.Checksum("abc12345TODOmakethisreal"),
		controlapi.WorkloadDescription("Workload published in devmode"),
	)
	if err != nil {
		return err
	}

	_, err = nodeClient.StartWorkload(request)
	if err != nil {
		return err
	}

	return nil
}

// All remaining funcs defined below are copypasta from nex/devrunner.go...

func uploadWorkload(nc *nats.Conn, devOpts models.DevRunOptions) (string, string, string, error) {
	js, err := nc.JetStream()
	if err != nil {
		panic(err)
	}
	var bucket nats.ObjectStore
	maxBytes := objectStoreMaxBytes
	if devOpts.DevBucketMaxBytes > 0 {
		maxBytes = devOpts.DevBucketMaxBytes
	}
	bucket, err = js.ObjectStore(objectStoreName)
	if err != nil {
		bucket, err = js.CreateObjectStore(&nats.ObjectStoreConfig{
			Bucket:      objectStoreName,
			Description: "Ad hoc object storage for NEX CLI developer mode uploads",
			MaxBytes:    int64(maxBytes),
		})
		if err != nil {
			return "", "", "", err
		}
	}
	bytes, err := os.ReadFile(devOpts.Filename)
	if err != nil {
		return "", "", "", err
	}
	key := filepath.Base(devOpts.Filename)
	key = strings.ReplaceAll(key, ".", "")

	_, err = bucket.PutBytes(key, bytes)
	if err != nil {
		return "", "", "", err
	}

	var workloadType string
	switch strings.Replace(filepath.Ext(devOpts.Filename), ".", "", 1) {
	case fileExtensionJS:
		workloadType = agentapi.NexExecutionProviderV8
	case fileExtensionWasm:
		workloadType = agentapi.NexExecutionProviderWasm
	default:
		workloadType = defaultWorkloadType
	}

	return fmt.Sprintf("nats://%s/%s", objectStoreName, key), key, workloadType, nil
}

func nexdir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	nexDir := path.Join(home, ".nex")
	return nexDir, os.MkdirAll(nexDir, defaultFileMode)
}

func readOrGenerateIssuer() (nkeys.KeyPair, error) {
	dir, err := nexdir()
	if err != nil {
		return nil, err
	}

	filename := path.Join(dir, "issuer.nk")
	bytes, err := os.ReadFile(filename)
	if errors.Is(err, fs.ErrNotExist) {
		return writeNewIssuer()
	} else {
		fmt.Printf("Reusing existing issuer account key: %s\n", filename)
		kp, err := nkeys.FromSeed(bytes)
		if err != nil {
			return nil, err
		}
		return kp, nil
	}
}

func readOrGeneratePublisher() (nkeys.KeyPair, error) {
	dir, err := nexdir()
	if err != nil {
		return nil, err
	}

	filename := path.Join(dir, "publisher.xk")
	bytes, err := os.ReadFile(filename)
	if errors.Is(err, fs.ErrNotExist) {
		return writeNewPublisher()
	} else {
		fmt.Printf("Reusing existing publisher xkey: %s\n", filename)
		kp, err := nkeys.FromCurveSeed(bytes)
		if err != nil {
			return nil, err
		}
		return kp, nil
	}
}

func writeNewPublisher() (nkeys.KeyPair, error) {
	dir, err := nexdir()
	if err != nil {
		return nil, err
	}

	kp, _ := nkeys.CreateCurveKeys()
	seed, _ := kp.Seed()
	publisherFile := path.Join(dir, "publisher.xk")
	err = os.WriteFile(publisherFile, seed, defaultFileMode)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Wrote new publisher xkey: %s\n", publisherFile)

	return kp, nil
}

func writeNewIssuer() (nkeys.KeyPair, error) {
	dir, err := nexdir()
	if err != nil {
		return nil, err
	}

	kp, _ := nkeys.CreateAccount()
	seed, _ := kp.Seed()
	issuerFile := path.Join(dir, "issuer.nk")
	err = os.WriteFile(issuerFile, seed, defaultFileMode)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Wrote new issuer account key: %s\n", issuerFile)

	return kp, nil
}
