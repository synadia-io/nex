package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	controlapi "github.com/synadia-io/nex/control-api"
	"github.com/synadia-io/nex/internal/models"
)

var (
	nexDir string
)

const (
	defaultFileMode     = os.FileMode(int(0770)) // owner and group r/w/x
	defaultWorkloadType = controlapi.NexWorkloadNative
	fileExtensionJS     = "js"
	fileExtensionWasm   = "wasm"

	objectStoreName     = "NEXCLIFILES"
	objectStoreMaxBytes = uint(100 * 1024 * 1024) // 100 MB
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	nexDir = path.Join(home, ".nex")
	err = os.MkdirAll(nexDir, defaultFileMode)
	if err != nil {
		panic(err)
	}
}

// Attempts to "deploy a file" by finding a suitable target and publishing the workload to an ad-hoc created bucket
// and using default issuer and publisher keys stored in ~/.nex. This should be as easy as typing "nex devrun ./amazingapp env1=foo env2=bar"
func RunDevWorkload(ctx context.Context, logger *slog.Logger) error {
	var os, arch string
	var err error

	if RunOpts.WorkloadType == "native" {
		os, arch, err = validateBinary(DevRunOpts.Filename)
		if err != nil {
			logger.Error("failed to validate binary", slog.Any("err", err))
		}
		logger.Debug("Binary validation complete", slog.String("os", os), slog.String("arch", arch))
	}

	nc, err := models.GenerateConnectionFromOpts(Opts, logger)
	if err != nil {
		return err
	}
	// developer mode can have a smaller discovery timeout, since we're assuming there's a NEX
	// node "nearby"
	nodeClient := controlapi.NewApiClientWithNamespace(nc, 750*time.Millisecond, Opts.Namespace, logger)

	target, err := randomNode(nodeClient, arch, os, RunOpts.WorkloadType)
	if err != nil {
		return err
	}

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
	workloadUrl, workloadName, err := uploadWorkload(nc, *DevRunOpts)
	if err != nil {
		return err
	}

	if RunOpts.WorkloadType == "v8" && len(RunOpts.TriggerSubjects) == 0 {
		return errors.New("cannot start a function-type workload without specifying at least one trigger subject")
	}

	if !slices.Contains([]string{"v8", "wasm", "native", "elf", "oci"}, workloadType) {
		fmt.Printf("Warning: You've supplied a custom workload type (%s). Target node must support this plugin", workloadType)
	}

	if DevRunOpts.AutoStop {
		for _, machine := range info.Machines {
			if machine.Workload.Name == workloadName {
				fmt.Printf("Workload %s (%s) already exists on the target. Attempting to stop it\n", workloadName, machine.Id)
				stopRequest, err := controlapi.NewStopRequest(target.NodeId, machine.Id, issuerKp)
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

	argv := []string{}
	if len(RunOpts.Argv) > 0 {
		argv = strings.Split(RunOpts.Argv, " ")
	}

	request, err := controlapi.NewDeployRequest(
		controlapi.Argv(argv),
		controlapi.Location(workloadUrl),
		controlapi.Environment(RunOpts.Env),
		controlapi.Essential(RunOpts.Essential),
		controlapi.Issuer(issuerKp),
		controlapi.SenderXKey(publisherXKey),
		controlapi.TargetNode(target.NodeId),
		controlapi.TargetPublicXKey(targetPublicXkey),
		controlapi.TriggerSubjects(RunOpts.TriggerSubjects),
		controlapi.WorkloadName(workloadName),
		controlapi.WorkloadType(RunOpts.WorkloadType),
		controlapi.Hash("abc12345TODOmakethisreal"),
		controlapi.WorkloadDescription("Workload published in devmode"),
	)
	if err != nil {
		return err
	}

	runResponse, err := nodeClient.StartWorkload(request)
	if err != nil {
		return err
	}
	renderRunResponse(target.NodeId, runResponse)

	return nil
}

func randomNode(nodeClient *controlapi.Client, arch, os string, workloadType controlapi.NexWorkload) (*controlapi.AuctionResponse, error) {
	candidates, err := auction(nodeClient, os, arch, workloadType)
	if err != nil {
		return nil, err
	}

	return &candidates[rand.Intn(len(candidates))], nil
}

func auction(nodeClient *controlapi.Client, os, arch string, workloadType controlapi.NexWorkload) ([]controlapi.AuctionResponse, error) {
	var _os, _arch *string
	if os != "" {
		_os = &os
	}
	if arch != "" {
		_arch = &arch
	}

	candidates, err := nodeClient.Auction(&controlapi.AuctionRequest{
		Arch:          _arch,
		OS:            _os,
		WorkloadTypes: []controlapi.NexWorkload{workloadType},
	})
	if err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, errors.New("unable to locate candidate node - no nodes discovered")
	}

	return candidates, nil
}

func uploadWorkload(nc *nats.Conn, devOpts models.DevRunOptions) (string, string, error) {
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
			return "", "", err
		}
	}
	bytes, err := os.ReadFile(devOpts.Filename)
	if err != nil {
		return "", "", err
	}
	key := filepath.Base(devOpts.Filename)
	key = strings.ReplaceAll(key, ".", "")

	_, err = bucket.PutBytes(key, bytes)
	if err != nil {
		return "", "", err
	}

	return fmt.Sprintf("nats://%s/%s", objectStoreName, key), key, nil
}

func readOrGenerateIssuer() (nkeys.KeyPair, error) {
	filename := path.Join(nexDir, "issuer.nk")
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
	filename := path.Join(nexDir, "publisher.xk")
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
	kp, _ := nkeys.CreateCurveKeys()
	seed, _ := kp.Seed()
	publisherFile := path.Join(nexDir, "publisher.xk")
	err := os.WriteFile(publisherFile, seed, defaultFileMode)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Wrote new publisher xkey: %s\n", publisherFile)

	return kp, nil
}

func writeNewIssuer() (nkeys.KeyPair, error) {
	kp, _ := nkeys.CreateAccount()
	seed, _ := kp.Seed()
	issuerFile := path.Join(nexDir, "issuer.nk")
	err := os.WriteFile(issuerFile, seed, defaultFileMode)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Wrote new issuer account key: %s\n", issuerFile)

	return kp, nil
}
