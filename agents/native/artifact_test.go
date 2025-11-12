package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/synadia-io/nex/_test"
)

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

func prepNatsObjStoreArtifact(t testing.TB, nc *nats.Conn, workingDir, binPath string) (string, error) {
	jsCtx, err := jetstream.New(nc)
	if err != nil {
		return "", err
	}

	obs, err := jsCtx.CreateOrUpdateObjectStore(context.TODO(), jetstream.ObjectStoreConfig{
		Bucket: "mybins",
	})
	if err != nil {
		return "", err
	}

	f, err := os.Open(binPath)
	if err != nil {
		return "", err
	}
	_, err = obs.Put(context.TODO(), jetstream.ObjectMeta{
		Name: "testnats_foo",
	}, f)
	if err != nil {
		return "", err
	}
	return "nats://mybins/testnats:foo", nil
}

func TestArtifactNats(t *testing.T) {
	tDir, err := os.MkdirTemp(os.TempDir(), "nex-test-working-dir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	binPath, binHash, binLen := createTestBinary(t, tDir)

	s := _test.StartNatsServer(t, tDir)
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)

	uri, err := prepNatsObjStoreArtifact(t, nc, tDir, binPath)
	if err != nil {
		t.Fatalf("Failed to prep OCI artifact: %v", err)
	}

	ref, err := getArtifact(uri, nc)
	if err != nil {
		t.Fatalf("Failed to get artifact: %v", err)
	}

	be.Equal(t, "testnats", ref.Name)
	be.Equal(t, "foo", ref.Tag)
	be.Equal(t, uri, ref.OriginalURI)
	be.Equal(t, binHash, ref.Digest)
	be.Equal(t, binLen, ref.Size)

	os.RemoveAll(tDir)
}

func TestArtifactFile(t *testing.T) {
	workingDir, err := os.MkdirTemp(os.TempDir(), "nex-test-working-dir")
	if err != nil {
		t.Errorf("Failed to create temp dir: %v", err)
	}

	binPath, binHash, binLen := createTestBinary(t, workingDir)

	uri := fmt.Sprintf("file://%s", binPath)
	ref, err := getArtifact(uri, nil)
	if err != nil {
		t.Errorf("Failed to get artifact: %v", err)
	}

	be.Equal(t, "output_binary", ref.Name)
	be.Zero(t, ref.Tag)
	be.Equal(t, uri, ref.OriginalURI)
	be.Equal(t, binHash, ref.Digest)
	be.Equal(t, binLen, ref.Size)

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
