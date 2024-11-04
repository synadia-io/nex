package actors

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	natsregistry "github.com/synadia-labs/oci-registry-nats"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

func startNatsServer(t testing.TB) (*server.Server, error) {
	t.Helper()

	s, err := server.NewServer(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	})

	if err != nil {
		return nil, err
	}

	go func() {
		if err := server.Run(s); err != nil {
			server.PrintAndDie(err.Error())
		}
	}()

	time.Sleep(1 * time.Second)

	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		return nil, err
	}
	jsCtx, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}
	_, err = jsCtx.CreateKeyValue(context.TODO(), jetstream.KeyValueConfig{
		Bucket: "registry",
	})
	if err != nil {
		return nil, err
	}
	nc.Close()

	return s, nil
}

func createTestBinary(t testing.TB, tmpDir string) string {
	t.Helper()

	f, err := os.Create(filepath.Join(tmpDir, "main.go"))
	if err != nil {
		t.Fatalf("Failed to create Go file: %v", err)
	}
	_, err = f.WriteString(testProg)
	if err != nil {
		t.Errorf("Failed to write to Go file: %v", err)
	}
	f.Close()

	// Command to compile the Go code into a binary
	cmd := exec.Command("go", "build", "-o", filepath.Join(tmpDir, "output_binary"), f.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command and check for errors
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to compile Go code: %v", err)
	}

	return filepath.Join(tmpDir, "output_binary")
}

func prepOCIArtifact(t testing.TB, workingDir string) (string, error) {
	t.Helper()
	s, err := startNatsServer(t)
	if err != nil {
		return "", err
	}

	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		return "", err
	}

	// find unused port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	nr, err := natsregistry.NewNatsRegistry(nc,
		natsregistry.WithWebserverAddress(fmt.Sprintf(":%d", port)),
		// natsregistry.WithLogger(slog.New(slog.NewTextHandler(os.Stdout, nil))),
	)
	if err != nil {
		return "", err
	}

	go func() {
		err = nr.Start()
		if err != nil {
			t.Error(err)
		}
	}()
	time.Sleep(1 * time.Second)

	repo, err := remote.NewRepository(fmt.Sprintf("localhost:%d/test", port))
	if err != nil {
		return "", err
	}
	repo.PlainHTTP = true
	repo.Client = &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
	}

	binPath := createTestBinary(t, workingDir)

	fs, err := file.New(t.TempDir())
	if err != nil {
		return "", err
	}
	defer fs.Close()
	ctx := context.Background()

	mediaType := "application/nex.artifact.binary"
	fileDescriptor, err := fs.Add(ctx, binPath, mediaType, "")
	if err != nil {
		return "", err
	}

	artifactType := "application/nex.artifact"
	opts := oras.PackManifestOptions{
		Layers: []v1.Descriptor{fileDescriptor},
	}
	manifestDescriptor, err := oras.PackManifest(ctx, fs, oras.PackManifestVersion1_1, artifactType, opts)
	if err != nil {
		return "", err
	}

	tag := "derp"
	if err = fs.Tag(ctx, manifestDescriptor, tag); err != nil {
		return "", err
	}

	_, err = oras.Copy(ctx, fs, tag, repo, tag, oras.DefaultCopyOptions)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("oci://localhost:%d/test:derp", port), nil
}

func prepNatsObjStoreArtifact(t testing.TB) string {
	return ""
}

func prepFileArtifact(t testing.TB, workingDir string) string {
	t.Helper()

	binary := createTestBinary(t, workingDir)
	return "file://" + binary
}

func TestOCIArtifact(t *testing.T) {
	workingDir := t.TempDir()
	uri, err := prepOCIArtifact(t, workingDir)
	if err != nil {
		t.Fatalf("Failed to prep OCI artifact: %v", err)
	}

	ref, err := getArtifact("testoci", uri)
	if err != nil {
		t.Fatalf("Failed to get artifact: %v", err)
	}
	defer os.Remove(ref.LocalCachePath)

	if ref.Name != "testoci" {
		t.Errorf("expected %s, got %s", "testoci", ref.Name)
	}
	if ref.Tag != "derp" {
		t.Errorf("expected %s, got %s", "derp", ref.Tag)
	}
	if ref.OriginalLocation.String() != uri {
		t.Errorf("expected %s, got %s", uri, ref.OriginalLocation.String())
	}
	if !strings.HasPrefix(ref.LocalCachePath, os.TempDir()+"/workload-") {
		t.Errorf("expected %s, got %s", os.TempDir()+"/workload-", ref.LocalCachePath)
	}
	// if ref.Digest != "b92aaf4823a9ffd5aa76f8af00708b60cd658d9dfbee39d717a566e57912204b" {
	// 	t.Errorf("expected %s, got %s", "b92aaf4823a9ffd5aa76f8af00708b60cd658d9dfbee39d717a566e57912204b", ref.Digest)
	// }
	if ref.Size != 2276655 {
		t.Errorf("expected %d, got %d", 2276655, ref.Size)
	}
}

func TestNatsArtifact(t *testing.T) {
	//startNatsServer(t)
	// Run the test
}

func TestFileArtifact(t *testing.T) {
	workingDir, err := os.MkdirTemp(os.TempDir(), "nex-test-working-dir")
	if err != nil {
		t.Errorf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(workingDir)

	uri := prepFileArtifact(t, workingDir)
	ref, err := getArtifact("test", uri)
	if err != nil {
		t.Errorf("Failed to get artifact: %v", err)
	}
	defer os.Remove(ref.LocalCachePath)

	if ref.Name != "test" {
		t.Errorf("expected %s, got %s", "test", ref.Name)
	}
	if ref.Tag != "latest" {
		t.Errorf("expected %s, got %s", "latest", ref.Tag)
	}
	if ref.OriginalLocation.String() != uri {
		t.Errorf("expected %s, got %s", uri, ref.OriginalLocation.String())
	}
	if !strings.HasPrefix(ref.LocalCachePath, os.TempDir()+"/workload-") {
		t.Errorf("expected %s, got %s", os.TempDir()+"/workload-", ref.LocalCachePath)
	}
	// if ref.Digest != "bcc1d22e206389acfcf479f2ae4f2042e063887296be45351576c8e3996cf483" {
	// 	t.Errorf("expected %s, got %s", "bcc1d22e206389acfcf479f2ae4f2042e063887296be45351576c8e3996cf483", ref.Digest)
	// }
	if ref.Size != 2276647 {
		t.Errorf("expected %d, got %d", 2276647, ref.Size)
	}
}

func TestTagCalculator(t *testing.T) {
	tests := []struct {
		uri              string
		expectedFilePath string
		expectedTag      string
		expectedError    error
	}{
		// tests latest tags
		{"file:///tmp/foo", "/tmp/foo", "latest", nil},
		{"oci://synadia/foo", "/foo", "latest", nil},
		{"nats://myobject/foo", "/foo", "latest", nil},
		// tests with a tag
		{"file:///tmp/foo:derp", "/tmp/foo", "derp", nil},
		{"oci://synadia/foo:derp", "/foo", "derp", nil},
		{"oci://synadia:8000/foo:derp", "/foo", "derp", nil},
		{"nats://myobject/foo:derp", "/foo", "derp", nil},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			u, err := url.Parse(tt.uri)
			if err != nil {
				t.Errorf("failed to parse url: %v", err)
			}
			file, tag := parsePathTag(u)
			if file != tt.expectedFilePath {
				t.Errorf("expected %s, got %s", tt.expectedFilePath, file)
			}
			if tag != tt.expectedTag {
				t.Errorf("expected %s, got %s", tt.expectedTag, tag)
			}
		})
	}
}

const testProg string = `package main

import (
    "fmt"
    "os"
    "os/signal"
    "time"
)

func main() {
    exit := make(chan os.Signal, 1)
    signal.Notify(exit, os.Interrupt)

    count := 0
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            count++
            fmt.Println("Count: ", count)
        case <-exit:
            fmt.Println("Exiting...")
            return
        }
    }
}`
