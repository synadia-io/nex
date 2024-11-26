package main

import (
	"archive/tar"
	"bytes"
	"context"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"

	"strings"

	"github.com/synadia-io/nex/api/nodecontrol"
	"github.com/synadia-io/nex/api/nodecontrol/gen"
	"github.com/synadia-io/nex/models"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/natscli/columns"
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
	Info   InfoWorkload   `cmd:"" help:"Get information about a workload"`
	Copy   CopyWorkload   `cmd:"" help:"Copy a workload to another node" aliases:"cp,clone"`
	Bundle BundleWorkload `cmd:"" help:"Bundles a workload into a compatable OCI image" aliases:"build,package"`
}

type NatsCreds struct {
	NatsUrl      string `placeholder:"nats://localhost:4222"`
	NatsUserJwt  string `placeholder:"ENATSJWTABC..."`
	NatsUserSeed string `placeholder:"SUAEY2LZ..."`
}

// Workload subcommands
type RunWorkload struct {
	NodeId      string            `help:"Node ID to run the workload on"`
	NodeTags    map[string]string `help:"Node tags to run the workload on; --node-id will take precedence"`
	NodePubXKey string            `name:"node-xkey-pub" help:"Node public xkey used for encryption"`

	WorkloadName             string            `name:"name" help:"Name of the workload"`
	WorkloadArguments        []string          `name:"argv" help:"Arguments to pass to the workload"`
	WorkloadEnvironment      map[string]string `name:"env" help:"Environment variables to set for the workload"`
	WorkloadDescription      string            `name:"description" help:"Description of the workload"`
	WorkloadHash             string            `name:"hash" help:"Hash of the workload. If provided, will use for validation, if not provided, will calculate"`
	WorkloadHostServiceCreds NatsCreds         `embed:"" prefix:"hostservices." help:"Host service configuration"`
	WorkloadJsDomain         string            `name:"jsdomain" help:"JS Domain to run the workload under"`
	WorkloadRetryCount       int               `name:"retry-count" help:"Number of times to retry the workload" default:"3"`
	WorkloadPublicKey        string            `name:"public-key" help:"Public key of the workload"`
	WorkloadUri              string            `name:"uri" help:"URI of the workload.  file:// oci:// nats://" placeholder:"oci://localhost:5000/workload:latest"`
	WorkloadTriggerSubjects  []string          `name:"triggers" help:"Subjects to trigger the workload"`
	WorkloadType             string            `name:"type" help:"Type of workload" default:"direct_start"`
	WorkloadRuntype          string            `name:"runtype" help:"Runtype of the workload: service, once, job" default:"service" enum:"service,once,job"`
}

func (RunWorkload) AfterApply(globals *Globals) error {
	return checkVer(globals)
}

func (r RunWorkload) Validate() error {
	var errs error

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

	if r.NodeId != "" {
		if r.NodePubXKey == "" {
			errs = errors.Join(errs, errors.New("If Node ID is provided, Node Public XKey is required"))
		}
	}

	if r.WorkloadRuntype == models.WorkloadRunTypeJob && len(r.WorkloadTriggerSubjects) == 0 {
		errs = errors.Join(errs, errors.New("Job workloads require at least one trigger subject"))
	}

	return errs
}

func (r RunWorkload) Run(ctx context.Context, globals *Globals, w *Workload) error {
	if globals.Check {
		return printTable("Run Workload Configuration", append(globals.Table(), r.Table()...)...)
	}

	if r.NodeTags == nil {
		r.NodeTags = map[string]string{}
	}

	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	controller, err := nodecontrol.NewControlApiClient(nc, slog.New(slog.NewTextHandler(os.Stdin, nil)))
	if err != nil {
		return err
	}

	startRequest := gen.StartWorkloadRequestJson{
		Argv:        r.WorkloadArguments,
		Description: r.WorkloadDescription,
		Hash:        r.WorkloadHash,
		HostServiceConfig: gen.SharedHostServiceJson{
			NatsUrl:      r.WorkloadHostServiceCreds.NatsUrl,
			NatsUserJwt:  r.WorkloadHostServiceCreds.NatsUserJwt,
			NatsUserSeed: r.WorkloadHostServiceCreds.NatsUserSeed,
		},
		Jsdomain:        r.WorkloadJsDomain,
		Namespace:       globals.Namespace,
		TargetPubXkey:   r.NodePubXKey,
		RetryCount:      r.WorkloadRetryCount,
		SenderPublicKey: r.WorkloadPublicKey,
		TriggerSubjects: r.WorkloadTriggerSubjects,
		Uri:             r.WorkloadUri,
		WorkloadName:    r.WorkloadName,
		WorkloadType:    r.WorkloadType,
		WorkloadRuntype: r.WorkloadRuntype,
	}

	txk, err := nkeys.CreateCurveKeys()
	if err != nil {
		return err
	}
	txk_pub, err := txk.PublicKey()
	if err != nil {
		return err
	}

	var env_b []byte
	if r.WorkloadEnvironment != nil {
		env_b, err = json.Marshal(r.WorkloadEnvironment)
		if err != nil {
			return err
		}
	}

	var resp *gen.StartWorkloadResponseJson
	if r.NodeId != "" {
		enc_env_b, err := txk.Seal(env_b, startRequest.TargetPubXkey)
		if err != nil {
			return err
		}
		b64_enc_env := base64.StdEncoding.EncodeToString(enc_env_b)
		startRequest.EncEnvironment = gen.SharedEncEnvJson{
			Base64EncryptedEnv: b64_enc_env,
			EncryptedBy:        txk_pub,
		}
		resp, err = controller.DeployWorkload(globals.Namespace, r.NodeId, startRequest)
		if err != nil {
			return err
		}
	} else {
		auctionResults, err := controller.Auction(globals.Namespace, r.NodeTags)
		if err != nil {
			return err
		}

		if len(auctionResults) == 0 {
			return errors.New("no nodes available for deployment")
		}

		nodeX := rand.IntN(len(auctionResults))
		bidderId := auctionResults[nodeX].BidderId

		// in an auction deploy, information about the target node is unknown, so we need to provide it in the auction
		startRequest.TargetPubXkey = auctionResults[nodeX].TargetXkey

		enc_env_b, err := txk.Seal(env_b, startRequest.TargetPubXkey)
		if err != nil {
			return err
		}
		b64_enc_env := base64.StdEncoding.EncodeToString(enc_env_b)
		startRequest.EncEnvironment = gen.SharedEncEnvJson{
			Base64EncryptedEnv: b64_enc_env,
			EncryptedBy:        txk_pub,
		}
		resp, err = controller.AuctionDeployWorkload(globals.Namespace, bidderId, startRequest)
		if err != nil {
			return err
		}
	}

	if resp.Started {
		if r.NodeId != "" {
			fmt.Printf("Workload %s [%s] started on node %s\n", r.WorkloadName, resp.Id, r.NodeId)
		} else {
			fmt.Printf("Workload %s [%s] started\n", r.WorkloadName, resp.Id)
		}
	} else {
		fmt.Printf("Workload %s failed to start\n", r.WorkloadName)
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

	resp, err := controller.UndeployWorkload(globals.Namespace, s.NodeId, globals.Namespace, req)
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

type InfoWorkload struct {
	WorkloadId   string `arg:"" description:"ID of the workload"`
	WorkloadType string `name:"type" description:"Type of workload" default:"direct_start"`

	Json bool `name:"json" description:"Output in JSON format" default:"false"`
}

func (InfoWorkload) AfterApply(globals *Globals) error {
	return checkVer(globals)
}

func (i InfoWorkload) Validate() error {
	var errs error

	return errs
}

func (i InfoWorkload) Run(ctx context.Context, globals *Globals) error {
	if globals.Check {
		return printTable("Build Workload Configuration", append(globals.Table(), i.Table()...)...)
	}

	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	controller, err := nodecontrol.NewControlApiClient(nc, slog.New(slog.NewTextHandler(os.Stdin, nil)))
	if err != nil {
		return err
	}

	resp, err := controller.FindWorkload(i.WorkloadType, globals.Namespace, i.WorkloadId)
	if errors.Is(err, nats.ErrTimeout) {
		fmt.Println("Workload not found")
		return nil
	}
	if err != nil {
		return err
	}

	if i.Json {
		out, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}

	w := columns.New("Information about Workload %s", i.WorkloadId)
	w.AddRow("Name", resp.WorkloadSummary.Name)
	w.AddRow("Start Time", resp.WorkloadSummary.StartTime)
	w.AddRowIf("Run Time", resp.WorkloadSummary.Runtime, resp.WorkloadSummary.Runtime != "")
	w.AddRow("Workload Type", resp.WorkloadSummary.WorkloadType)
	w.AddRow("Workload State", resp.WorkloadSummary.WorkloadState)
	s, err := w.Render()
	if err != nil {
		return err
	}
	fmt.Println(s)
	return nil
}

type CopyWorkload struct {
	WorkloadId   string `arg:"" description:"ID of the workload"`
	WorkloadType string `name:"type" description:"Type of workload" default:"direct_start"`
	StopOriginal bool   `name:"stop" description:"Stop the original workload after copying" default:"false"`

	NodeId   string            `description:"Node ID of target workload. If not provided, auction is preformed"`
	NodeXkey string            `description:"Node public xkey used for encryption"`
	NodeTags map[string]string `description:"Node tags to use during auction; --node-id will take precedence"`
}

func (CopyWorkload) AfterApply(globals *Globals) error {
	return checkVer(globals)
}

func (c CopyWorkload) Validate() error {
	var errs error

	if c.NodeId != "" && c.NodeXkey == "" {
		errs = errors.Join(errs, errors.New("Node public xkey is required if Node ID is provided"))
	}

	return errs
}

func (c CopyWorkload) Run(ctx context.Context, globals *Globals) error {
	if globals.Check {
		return printTable("Copy Workload Configuration", append(globals.Table(), c.Table()...)...)
	}
	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	controller, err := nodecontrol.NewControlApiClient(nc, slog.New(slog.NewTextHandler(os.Stdin, nil)))
	if err != nil {
		return err
	}

	if c.NodeId != "" {
		resp, err := controller.CopyWorkload(c.WorkloadId, globals.Namespace, c.NodeXkey)
		if err != nil {
			return err
		}

		startResp, err := controller.DeployWorkload(globals.Namespace, c.NodeId, *resp)
		if err != nil {
			return err
		}

		fmt.Printf("Workload successfully copied! New workloadId: %s\n", startResp.Id)
		return nil
	}

	auctionResults, err := controller.Auction(globals.Namespace, c.NodeTags)
	if err != nil {
		return err
	}

	nodeX := rand.IntN(len(auctionResults))
	resp, err := controller.CopyWorkload(c.WorkloadId, globals.Namespace, auctionResults[nodeX].TargetXkey)
	if err != nil {
		return err
	}

	startResp, err := controller.AuctionDeployWorkload(globals.Namespace, auctionResults[nodeX].BidderId, *resp)
	if err != nil {
		return err
	}

	if c.StopOriginal {
		// controller.UndeployWorkload(c.WorkloadId)
		_ = 0 // need to stop c.WorkloadId here
	}

	fmt.Printf("Workload successfully copied! New workloadId: %s\n", startResp.Id)
	return nil
}

type BundleWorkload struct {
	Binaries []string `help:"Binary to package"`
	OS       string   `help:"Operating system of the binary" enum:"linux,darwin" default:"linux"`
	Arch     string   `help:"Architecture of the binary" enum:"amd64,arm64" default:"amd64"`
	Output   string   `help:"Output file name" default:"./artifact.tar"`

	Push                bool   `help:"Push the workload to the registry"`
	Registry            string `help:"Registry to push the workload to" default:"ghcr.io"`
	RegistryUser        string `help:"Registry username"`
	RegistryPassword    string `help:"Registry password"`
	RegistryHttp        bool   `name:"plain-http" help:"Use http instead of https for registry"`
	WorkloadName        string `name:"name" help:"Name of the workload"`
	WorkloadTag         string `name:"tag" help:"Tag of the workload" default:"latest"`
	WorkloadDescription string `name:"description" help:"Description of the workload"`
	WorkloadSigningKey  string `name:"public-key" help:"Public key of the workload. OCI layers will be signed with this key" default:"${defaultResourcePath}/issuer.nk"`
	WorkloadType        string `name:"type" help:"Type of workload" default:"direct_start"`
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

	layers := []v1.Descriptor{}
	store := memory.New()

	for _, bin := range b.Binaries {
		binFile, err := os.Open(bin)
		if err != nil {
			return err
		}
		defer binFile.Close()

		os, arch, err := getBinPair(bin)
		if err != nil {
			return err
		}

		_, err = binFile.Seek(0, 0)
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
			Annotations: map[string]string{
				"os":   os,
				"arch": arch,
			},
		}

		err = store.Push(ctx, fileDescriptor, bytes.NewBuffer(b_b))
		if err != nil {
			return err
		}

		err = store.Tag(ctx, fileDescriptor, string(digest.FromBytes(b_b)))
		if err != nil {
			return err
		}

		layers = append(layers, fileDescriptor)
	}

	opts := oras.PackManifestOptions{
		Layers: layers,
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

		if b.RegistryHttp {
			repo.PlainHTTP = true
		}

		var credFunc auth.CredentialFunc
		if b.RegistryUser == "" && b.RegistryPassword == "" {
			credFunc = auth.StaticCredential(b.Registry, auth.Credential{
				Username: b.RegistryUser,
				Password: b.RegistryPassword,
			})
		} else {
			credFunc = nil
		}

		repo.Client = &auth.Client{
			Client:     retry.DefaultClient,
			Cache:      auth.NewCache(),
			Credential: credFunc,
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

func getBinPair(filePath string) (string, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	// Check for ELF (Linux)
	if elfFile, err := elf.NewFile(file); err == nil {
		var arch string
		switch elfFile.Machine {
		case elf.EM_ARM:
			arch = "arm"
		case elf.EM_AARCH64:
			arch = "arm64"
		case elf.EM_386:
			arch = "386"
		case elf.EM_X86_64:
			arch = "amd64"
		default:
			arch = "unknown"
		}
		return "linux", arch, nil
	}

	// Reset the file to the beginning for the next check
	_, err = file.Seek(0, 0)
	if err != nil {
		return "", "", err
	}

	// Check for Mach-O (macOS)
	if machoFile, err := macho.NewFile(file); err == nil {
		var arch string
		switch machoFile.Cpu {
		case macho.Cpu386:
			arch = "386"
		case macho.CpuAmd64:
			arch = "amd64"
		case macho.CpuArm:
			arch = "arm"
		case macho.CpuArm64:
			arch = "arm64"
		default:
			arch = "unknown"
		}
		return "darwin", arch, nil
	}

	// Reset the file to the beginning for the next check
	_, err = file.Seek(0, 0)
	if err != nil {
		return "", "", err
	}

	// Check for PE (Windows)
	if peFile, err := pe.NewFile(file); err == nil {
		var arch string
		switch peFile.Machine {
		case pe.IMAGE_FILE_MACHINE_I386:
			arch = "386"
		case pe.IMAGE_FILE_MACHINE_AMD64:
			arch = "amd64"
		case pe.IMAGE_FILE_MACHINE_ARM:
			arch = "arm"
		case pe.IMAGE_FILE_MACHINE_ARM64:
			arch = "arm64"
		default:
			arch = "unknown"
		}
		return "windows", arch, nil
	}

	return "", "", nil
}
