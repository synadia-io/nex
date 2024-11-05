package actors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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

func createTestBinary(t testing.TB, tmpDir string) (string, string, int) {
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
	// go build -trimpath -ldflags="-buildid= -X 'main.buildTime=static_time'"
	cmd := exec.Command("go", "build", "-trimpath", "-o", filepath.Join(tmpDir, "output_binary"), f.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command and check for errors
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to compile Go code: %v", err)
	}

	f, err = os.Open(filepath.Join(tmpDir, "output_binary"))
	if err != nil {
		t.Fatalf("Failed to open binary: %v", err)
	}

	f_b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Failed to read binary: %v", err)
	}

	hash := sha256.New()
	_, err = hash.Write(f_b)
	if err != nil {
		t.Fatalf("Failed to hash binary: %v", err)
	}

	return filepath.Join(tmpDir, "output_binary"), hex.EncodeToString(hash.Sum(nil)), len(f_b)
}

func prepOCIArtifact(t testing.TB, workingDir, binPath string) (string, error) {
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

	fs, err := file.New(t.TempDir())
	if err != nil {
		return "", err
	}
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

	fs.Close()
	return fmt.Sprintf("oci://localhost:%d/test:derp", port), nil
}

func prepNatsObjStoreArtifact(t testing.TB, workingDir, binPath string) (string, *nats.Conn, error) {
	s, err := startNatsServer(t)
	if err != nil {
		return "", nil, err
	}

	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		return "", nil, err
	}

	jsCtx, err := jetstream.New(nc)
	if err != nil {
		return "", nil, err
	}

	obs, err := jsCtx.CreateOrUpdateObjectStore(context.TODO(), jetstream.ObjectStoreConfig{
		Bucket: "mybins",
	})
	if err != nil {
		return "", nil, err
	}

	f, err := os.Open(binPath)
	if err != nil {
		return "", nil, err
	}
	_, err = obs.Put(context.TODO(), jetstream.ObjectMeta{
		Name: "testnats_foo",
	}, f)
	if err != nil {
		return "", nil, err
	}

	return "nats://mybins/testnats:foo", nc, nil
}

func prepFileArtifact(t testing.TB, workingDir, binPath string) string {
	t.Helper()

	return "file://" + binPath
}

func TestOCIArtifact(t *testing.T) {
	workingDir := t.TempDir()
	binPath, binHash, binLen := createTestBinary(t, workingDir)

	uri, err := prepOCIArtifact(t, workingDir, binPath)
	if err != nil {
		t.Fatalf("Failed to prep OCI artifact: %v", err)
	}

	ref, err := getArtifact("testoci", uri, nil)
	if err != nil {
		t.Fatalf("Failed to get artifact: %v", err)
	}

	if ref.Name != "testoci" {
		t.Errorf("expected %s, got %s", "testoci", ref.Name)
	}
	if ref.Tag != "derp" {
		t.Errorf("expected %s, got %s", "derp", ref.Tag)
	}
	if ref.OriginalLocation.String() != uri {
		t.Errorf("expected %s, got %s", uri, ref.OriginalLocation.String())
	}
	if !strings.HasPrefix(ref.LocalCachePath, filepath.Join(os.TempDir(), "workload-")) {
		t.Errorf("expected %s, got %s", filepath.Join(os.TempDir(), "/workload-"), ref.LocalCachePath)
	}
	if ref.Digest != binHash {
		t.Errorf("expected %s, got %s", binHash, ref.Digest)
	}
	if ref.Size != binLen {
		t.Errorf("expected %d, got %d", binLen, ref.Size)
	}

	os.Remove(ref.LocalCachePath)
}

func TestNatsArtifact(t *testing.T) {
	workingDir := t.TempDir()
	binPath, binHash, binLen := createTestBinary(t, workingDir)

	uri, nc, err := prepNatsObjStoreArtifact(t, workingDir, binPath)
	if err != nil {
		t.Fatalf("Failed to prep OCI artifact: %v", err)
	}

	ref, err := getArtifact("testnats", uri, nc)
	if err != nil {
		t.Fatalf("Failed to get artifact: %v", err)
	}

	if ref.Name != "testnats" {
		t.Errorf("expected %s, got %s", "testnats", ref.Name)
	}
	if ref.Tag != "foo" {
		t.Errorf("expected %s, got %s", "foo", ref.Tag)
	}
	if ref.OriginalLocation.String() != uri {
		t.Errorf("expected %s, got %s", uri, ref.OriginalLocation.String())
	}
	if !strings.HasPrefix(ref.LocalCachePath, filepath.Join(os.TempDir(), "workload-")) {
		t.Errorf("expected %s, got %s", filepath.Join(os.TempDir(), "workload-"), ref.LocalCachePath)
	}
	if ref.Digest != binHash {
		t.Errorf("expected %s, got %s", binHash, ref.Digest)
	}
	if ref.Size != binLen {
		t.Errorf("expected %d, got %d", binLen, ref.Size)
	}

	os.Remove(ref.LocalCachePath)
}

func TestFileArtifact(t *testing.T) {

	workingDir, err := os.MkdirTemp(os.TempDir(), "nex-test-working-dir")
	if err != nil {
		t.Errorf("Failed to create temp dir: %v", err)
	}

	binPath, binHash, binLen := createTestBinary(t, workingDir)

	uri := prepFileArtifact(t, workingDir, binPath)
	ref, err := getArtifact("test", uri, nil)
	if err != nil {
		t.Errorf("Failed to get artifact: %v", err)
	}

	if ref.Name != "test" {
		t.Errorf("expected %s, got %s", "test", ref.Name)
	}
	if ref.Tag != "latest" {
		t.Errorf("expected %s, got %s", "latest", ref.Tag)
	}
	if ref.OriginalLocation.String() != uri {
		t.Errorf("expected %s, got %s", uri, ref.OriginalLocation.String())
	}
	if !strings.HasPrefix(ref.LocalCachePath, filepath.Join(os.TempDir(), "workload-")) {
		t.Errorf("expected %s, got %s", filepath.Join(os.TempDir(), "workload-"), ref.LocalCachePath)
	}
	if ref.Digest != binHash {
		t.Errorf("expected %s, got %s", binHash, ref.Digest)
	}
	if ref.Size != binLen {
		t.Errorf("expected %d, got %d", binLen, ref.Size)
	}

	os.Remove(ref.LocalCachePath)
	os.RemoveAll(workingDir)
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
