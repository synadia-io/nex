package actors

import (
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func createTestBinary(t testing.TB) string {
	t.Helper()

	f, err := os.Create(filepath.Join(os.TempDir(), "main.go"))
	if err != nil {
		t.Fatalf("Failed to create Go file: %v", err)
	}
	f.WriteString(testProg)
	f.Close()

	// Command to compile the Go code into a binary
	cmd := exec.Command("go", "build", "-o", filepath.Join(t.TempDir(), "output_binary"), f.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command and check for errors
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to compile Go code: %v", err)
	}

	return f.Name()
}

func startOCIRegistry(t testing.TB) string {
	t.Helper()
	// Start the OCI registry
	return ""
}

func startNatsServer(t testing.TB) string {
	t.Helper()
	// Start the NATS server
	return ""
}

func prepFileArtifact(t testing.TB) string {
	t.Helper()

	binary := createTestBinary(t)
	return "file://" + binary
}

func TestOCIArtiface(t *testing.T) {
	startOCIRegistry(t)
	// Run the test
}

func TestNatsArtifact(t *testing.T) {
	startNatsServer(t)
	// Run the test
}

func TestFileArtifact(t *testing.T) {
	uri := prepFileArtifact(t)
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
	if ref.Digest != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("expected %s, got %s", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", ref.Digest)
	}
	if ref.Size != 0 {
		t.Errorf("expected %d, got %d", 0, ref.Size)
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
