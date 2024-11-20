package actors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func startNatsServer(t testing.TB) (*server.Server, error) {
	t.Helper()

	s, err := server.NewServer(&server.Options{
		Port:      -1,
		JetStream: true,
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
	t.Log("not implemented")
}

func TestNatsArtifact(t *testing.T) {
	tDir, err := os.MkdirTemp(os.TempDir(), "nex-test-working-dir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	binPath, binHash, binLen := createTestBinary(t, tDir)

	uri, nc, err := prepNatsObjStoreArtifact(t, tDir, binPath)
	if err != nil {
		t.Fatalf("Failed to prep OCI artifact: %v", err)
	}

	ref, err := GetArtifact("testnats", uri, nc)
	if err != nil {
		t.Fatalf("Failed to get artifact: %v", err)
	}

	if ref.Name != "testnats" {
		t.Errorf("expected %s, got %s", "testnats", ref.Name)
	}
	if ref.Tag != "foo" {
		t.Errorf("expected %s, got %s", "foo", ref.Tag)
	}
	if ref.OriginalLocation != uri {
		t.Errorf("expected %s, got %s", uri, ref.OriginalLocation)
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
	os.RemoveAll(tDir)
}

func TestFileArtifact(t *testing.T) {
	workingDir, err := os.MkdirTemp(os.TempDir(), "nex-test-working-dir")
	if err != nil {
		t.Errorf("Failed to create temp dir: %v", err)
	}

	binPath, binHash, binLen := createTestBinary(t, workingDir)

	uri := prepFileArtifact(t, workingDir, binPath)
	ref, err := GetArtifact("test", uri, nil)
	if err != nil {
		t.Errorf("Failed to get artifact: %v", err)
	}

	if ref.Name != "test" {
		t.Errorf("expected %s, got %s", "test", ref.Name)
	}
	if ref.Tag != "" {
		t.Errorf("expected no tag, got %s", ref.Tag)
	}
	if ref.OriginalLocation != uri {
		t.Errorf("expected %s, got %s", uri, ref.OriginalLocation)
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
