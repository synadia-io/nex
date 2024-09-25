package internalnats

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nuid"
)

func TestInternalNatsServer(t *testing.T) {
	server, err := NewInternalNatsServer(slog.Default(), filepath.Join(os.TempDir(), "pnats-test"), true, false)
	if err != nil {
		t.Fatalf("Failed to create internal nats server: %s", err)
	}
	serverUrl := server.Connection().Servers()[0]
	sUrl, err := url.Parse(serverUrl)
	if err != nil {
		t.Fatalf("Should have a valid URL from the server but didn't: %s", err)
	}
	p := server.Port()
	if fmt.Sprintf("%d", p) != sUrl.Port() {
		t.Fatalf("Port number from options doesn't match what's running: %d %s", p, sUrl.Port())
	}

	workloadId := nuid.Next()

	keypair, err := server.CreateCredentials(workloadId)
	if err != nil {
		t.Fatalf("Should have been able to add a workload user but couldn't: %s", err)
	}
	pk, _ := keypair.PublicKey()
	seed, _ := keypair.Seed()
	fmt.Printf("New workload user: %s %s\n", pk, string(seed))

	// log in as the new workload
	_, err = nats.Connect(server.Connection().Servers()[0], nats.Nkey(pk, func(b []byte) ([]byte, error) {
		return keypair.Sign(b)
	}))

	if err != nil {
		t.Fatalf("Couldn't connect to the internal server as a workload: %s", err)
	}

	workloadId2 := nuid.Next()
	keypair2, err := server.CreateCredentials(workloadId2)
	if err != nil {
		t.Fatalf("Should have been able to add a workload user but couldn't: %s", err)
	}
	pk2, _ := keypair2.PublicKey()
	seed2, _ := keypair2.Seed()
	fmt.Printf("New workload user: %s %s\n", pk2, string(seed2))

	// log in as the new workload
	_, err = nats.Connect(server.Connection().Servers()[0], nats.Nkey(pk2, func(b []byte) ([]byte, error) {
		return keypair2.Sign(b)
	}))

	if err != nil {
		t.Fatalf("Couldn't connect to the internal server as a workload: %s", err)
	}
}

// With the new security system, all agents will simply pull their workload binary from
// the NEXCACHE bucket in their account, with the key of 'workload'
func TestInternalNatsServerFileCache(t *testing.T) {
	ctx := context.Background()
	server, err := NewInternalNatsServer(slog.Default(), filepath.Join(os.TempDir(), "pnats-test"), true, false)
	if err != nil {
		t.Fatalf("Failed to create internal nats server: %s", err)
	}
	fmt.Printf("Internal server on %s\n", server.Connection().Servers()[0])

	workloadId := nuid.Next()

	keypair, err := server.CreateCredentials(workloadId)
	if err != nil {
		t.Fatalf("Should have been able to add a workload user but couldn't: %s", err)
	}
	pk, _ := keypair.PublicKey()
	seed, _ := keypair.Seed()
	fmt.Printf("New workload user: %s %s\n", pk, string(seed))

	_, err = server.StoreFileForID(workloadId, "filename", []byte{1, 2, 3, 4, 5, 6, 7, 8, 9})
	if err != nil {
		t.Fatalf("Should have gotten no error but didn't: %s", err)
	}

	ud, _ := server.FindCredentials(workloadId)
	userCn, _ := server.ConnectionWithCredentials(ud)
	js, _ := jetstream.New(userCn)
	bucket, _ := js.ObjectStore(ctx, workloadCacheBucketName)
	workload, err := bucket.GetBytes(ctx, "filename")
	if err != nil {
		t.Fatalf("Should have queried the workload bytes, but got error instead: %s", err)
	}

	if !slices.Equal(workload, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}) {
		t.Fatalf("File bytes did not round trip properly, got %+v", workload)
	}
}
