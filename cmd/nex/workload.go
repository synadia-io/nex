package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	//"path/filepath"
	"strings"

	"github.com/synadia-io/nex/api/nodecontrol"
	"github.com/synadia-io/nex/api/nodecontrol/gen"

	"github.com/nats-io/nkeys"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

const (
	NexOCIArtifactTypePrefix string = "application/nex."
	NexOCIManifestType       string = "application/nex-workload"
)

var validURIPrefix []string = []string{"nats://", "file://", "oci://"}

type Workload struct {
	Run    RunWorkload    `cmd:"" help:"Run a workload on a target node" aliases:"start,deploy"`
	Stop   StopWorkload   `cmd:"" help:"Stop a running workload" aliases:"undeploy"`
	Bundle BundleWorkload `cmd:"" help:"Bundles a workload into a compatable OCI image" aliases:"build,package"`
}

type NatsCreds struct {
	NatsUrl      string `placeholder:"nats://localhost:4222"`
	NatsUserJwt  string `placeholder:"ENATSJWTABC..."`
	NatsUserSeed string `placeholder:"SUAEY2LZ..."`
}

// Workload subcommands
type RunWorkload struct {
	NodeId string `description:"Node ID to run the workload on"`

	WorkloadName             string            `name:"name" description:"Name of the workload"`
	WorkloadArguments        []string          `name:"argv" description:"Arguments to pass to the workload"`
	WorkloadEnvironment      map[string]string `name:"env" description:"Environment variables to set for the workload"`
	WorkloadDescription      string            `name:"description" description:"Description of the workload"`
	WorkloadEssential        bool              `name:"essential" description:"Is the workload essential" default:"false"`
	WorkloadHash             string            `name:"hash" description:"Hash of the workload. If provided, will use for validation, if not provided, will calculate"`
	WorkloadHostServiceCreds NatsCreds         `embed:"" prefix:"hostservices." description:"Host service configuration"`
	WorkloadJsDomain         string            `name:"jsdomain" description:"JS Domain to run the workload under"`
	WorkloadRetryCount       int               `name:"retry-count" description:"Number of times to retry the workload" default:"3"`
	WorkloadPublicKey        string            `name:"public-key" description:"Public key of the workload"`
	WorkloadUri              string            `name:"uri" description:"URI of the workload.  file:// oci:// nats://" placeholder:"file://./workload"`
	WorkloadTriggerSubjects  []string          `name:"triggers" description:"Subjects to trigger the workload"`
	WorkloadType             string            `name:"type" description:"Type of workload" default:"direct_start"`
}

func (RunWorkload) AfterApply(globals *Globals) error {
	return checkVer(globals)
}

func (r RunWorkload) Validate() error {
	var errs error

	if !nkeys.IsValidPublicServerKey(r.NodeId) {
		errs = errors.Join(errs, errors.New("Node ID is not a valid public server key"))
	}

	if r.WorkloadUri == "" {
		errs = errors.Join(errs, errors.New("Workload URI is required"))
	}

	validPrefix := false
	for _, prefix := range validURIPrefix {
		if strings.HasPrefix(r.WorkloadUri, prefix) {
			validPrefix = true
			break
		}
	}
	if !validPrefix {
		errs = errors.Join(errs, errors.New("Workload URI must start with one of the following prefixes: "+strings.Join(validURIPrefix, ", ")))
	}

	return errs
}

func (r RunWorkload) Run(ctx context.Context, globals *Globals, w *Workload) error {
	if globals.Check {
		return printTable("Run Workload Configuration", append(globals.Table(), r.Table()...)...)
	}

	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	controller, err := nodecontrol.NewControlApiClient(nc, slog.New(slog.NewTextHandler(os.Stdin, nil)))
	if err != nil {
		return err
	}

	var env string
	if r.WorkloadEnvironment != nil {
		env_b, err := json.Marshal(r.WorkloadEnvironment)
		if err != nil {
			return err
		}
		env = base64.StdEncoding.EncodeToString(env_b)
	}

	startRequest := gen.StartWorkloadRequestJson{
		Argv:        r.WorkloadArguments,
		Description: r.WorkloadDescription,
		Environment: env,
		Essential:   r.WorkloadEssential,
		Hash:        r.WorkloadHash,
		HostServiceConfig: gen.HostServicesConfig{
			NatsUrl:      r.WorkloadHostServiceCreds.NatsUrl,
			NatsUserJwt:  r.WorkloadHostServiceCreds.NatsUserJwt,
			NatsUserSeed: r.WorkloadHostServiceCreds.NatsUserSeed,
		},
		Jsdomain:        r.WorkloadJsDomain,
		RetryCount:      r.WorkloadRetryCount,
		SenderPublicKey: r.WorkloadPublicKey,
		TriggerSubjects: r.WorkloadTriggerSubjects,
		Uri:             r.WorkloadUri,
		WorkloadName:    r.WorkloadName,
		WorkloadType:    r.WorkloadType,
	}

	resp, err := controller.DeployWorkload(globals.Namespace, r.NodeId, startRequest)
	if err != nil {
		return err
	}

	if resp.Started {
		fmt.Printf("Workload %s [%s] started on node %s\n", r.WorkloadName, resp.Id, r.NodeId)
	} else {
		fmt.Printf("Workload %s failed to start on node %s\n", r.WorkloadName, r.NodeId)
	}

	return nil
}

type StopWorkload struct {
	NodeId       string `description:"Node ID of the node the workload in running"`
	WorkloadId   string `description:"ID of the workload to stop" required:"true"`
	WorkloadType string `name:"type" description:"Type of workload" default:"direct_start"`
}

func (StopWorkload) AfterApply(globals *Globals) error {
	return checkVer(globals)
}

func (s StopWorkload) Validate() error {
	var errs error
	return errs
}

func (s StopWorkload) Run(ctx context.Context, globals *Globals, w *Workload) error {
	if globals.Check {
		return printTable("Stop Workload Configuration", append(globals.Table(), s.Table()...)...)
	}

	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	controller, err := nodecontrol.NewControlApiClient(nc, slog.New(slog.NewTextHandler(os.Stdin, nil)))
	if err != nil {
		return err
	}

	req := gen.StopWorkloadRequestJson{
		NodeId:       s.NodeId,
		WorkloadId:   s.WorkloadId,
		WorkloadType: s.WorkloadType,
	}

	resp, err := controller.UndeployWorkload(s.NodeId, globals.Namespace, req)
	if err != nil {
		return err
	}

	if resp.Stopped {
		fmt.Printf("Workload %s stopped on node %s\n", s.WorkloadId, s.NodeId)
	} else {
		fmt.Printf("Workload %s failed to stop on node %s\n", s.WorkloadId, s.NodeId)
	}

	return nil
}

type BundleWorkload struct {
	Binary string `description:"Binary to package"`
	OS     string `description:"Operating system of the binary" enum:"linux,darwin" default:"linux"`
	Arch   string `description:"Architecture of the binary" enum:"amd64,arm64" default:"amd64"`
	Output string `description:"Output file name" default:"./artifact.tar"`

	Push                bool   `description:"Push the workload to the registry"`
	Registry            string `description:"Registry to push the workload to" default:"ghcr.io"`
	RegistryUser        string `description:"Registry username"`
	RegistryPassword    string `description:"Registry password"`
	WorkloadName        string `name:"name" description:"Name of the workload"`
	WorkloadTag         string `name:"tag" description:"Tag of the workload" default:"latest"`
	WorkloadDescription string `name:"description" description:"Description of the workload"`
	WorkloadSigningKey  string `name:"public-key" description:"Public key of the workload. OCI layers will be signed with this key" default:"${defaultResourcePath}/issuer.nk"`
	WorkloadType        string `name:"type" description:"Type of workload" default:"direct_start"`
}

func (BundleWorkload) AfterApply(globals *Globals) error {
	return checkVer(globals)
}

func (b BundleWorkload) Validate() error {
	var errs error

	if i, err := os.Stat(b.WorkloadSigningKey); err != nil || i.IsDir() {
		errs = errors.Join(errs, errors.New("Public key file does not exist"))
	}

	return errs
}

func (b BundleWorkload) Run(ctx context.Context, globals *Globals) error {
	if globals.Check {
		return printTable("Build Workload Configuration", append(globals.Table(), b.Table()...)...)
	}

	binFile, err := os.Open(b.Binary)
	if err != nil {
		return err
	}

	b_b, err := io.ReadAll(binFile)
	if err != nil {
		return err
	}

	detectWith := b_b
	if len(b_b) >= 512 {
		detectWith = b_b[:512]
	}

	fileDescriptor := v1.Descriptor{
		MediaType:    http.DetectContentType(detectWith),
		ArtifactType: NexOCIArtifactTypePrefix + b.WorkloadType,
		Digest:       digest.FromBytes(b_b),
		Size:         int64(len(b_b)),
	}

	store := memory.New()
	err = store.Push(ctx, fileDescriptor, bytes.NewBuffer(b_b))
	if err != nil {
		return err
	}

	err = store.Tag(ctx, fileDescriptor, string(digest.FromBytes(b_b)))
	if err != nil {
		return err
	}

	opts := oras.PackManifestOptions{
		Layers: []v1.Descriptor{fileDescriptor},
	}

	manifestDescriptor, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, NexOCIManifestType, opts)
	if err != nil {
		return err
	}

	if err = store.Tag(ctx, manifestDescriptor, b.WorkloadTag); err != nil {
		return err
	}

	// prepare artifact
	tDir, err := os.MkdirTemp(os.TempDir(), "nex-artifact-builder-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tDir)

	f, err := os.Create(filepath.Join(tDir, "oci-layout"))
	if err != nil {
		return err
	}
	_, err = f.WriteString(`{"imageLayoutVersion":"1.0.0"}`)
	if err != nil {
		return err
	}
	f.Close()

	rc, err := store.Fetch(ctx, manifestDescriptor)
	if err != nil {
		return err
	}
	defer rc.Close()
	manifestRaw, _ := io.ReadAll(rc)
	f, err = os.Create(filepath.Join(tDir, "index.json"))
	if err != nil {
		return err
	}
	_, err = f.Write(manifestRaw)
	if err != nil {
		return err
	}
	f.Close()

	var manifest v1.Manifest
	err = json.Unmarshal(manifestRaw, &manifest)
	if err != nil {
		return err
	}

	for _, l := range manifest.Layers {
		rc, err := store.Fetch(ctx, l)
		if err != nil {
			return err
		}
		defer rc.Close()
		algDig := strings.Split(l.Digest.String(), ":")
		if len(algDig) != 2 {
			return errors.New("Invalid digest")
		}

		bPath := filepath.Join(tDir, fmt.Sprintf("blobs/%s/%s", algDig[0], algDig[1]))

		err = os.MkdirAll(filepath.Dir(bPath), 0755)
		if err != nil {
			return err
		}

		f, err := os.Create(bPath)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, rc)
		if err != nil {
			return err
		}
		f.Close()
	}

	if b.Push {
		repo, err := remote.NewRepository(fmt.Sprintf("%s/%s", b.Registry, b.WorkloadName))
		if err != nil {
			return err
		}
		repo.Client = &auth.Client{
			Client: retry.DefaultClient,
			Cache:  auth.NewCache(),
			Credential: auth.StaticCredential(b.Registry, auth.Credential{
				Username: b.RegistryUser,
				Password: b.RegistryPassword,
			}),
		}

		_, err = oras.Copy(ctx, store, b.WorkloadTag, repo, b.WorkloadTag, oras.DefaultCopyOptions)
		if err != nil {
			return err
		}

		fmt.Printf("Workload push successful: %s/%s:%s\n", b.Registry, b.WorkloadName, b.WorkloadTag)
		return nil
	}
	// tar temp directory and move artifact to b.Output
	err = os.MkdirAll(filepath.Dir(b.Output), 0755)
	if err != nil {
		return err
	}

	f, err = os.Create(b.Output)
	if err != nil {
		return err
	}
	defer f.Close()

	tw := tar.NewWriter(f)
	defer tw.Close()

	err = filepath.Walk(tDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create a header
		header, err := tar.FileInfoHeader(info, path)
		if err != nil {
			return err
		}

		// Update the header name to be relative to the source directory
		//	header.Name, _ = filepath.Rel(filepath.Base(b.Output), path)
		header.Name, _ = filepath.Rel(tDir, path)

		// Write the header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// If it's a directory, skip writing content
		if info.IsDir() {
			return nil
		}

		// Open the file for reading
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Copy the file content to the tar writer
		_, err = io.Copy(tw, file)
		return err
	})

	if err != nil {
		return err
	}

	return nil
}
