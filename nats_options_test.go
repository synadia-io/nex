package nex

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-labs/nex/models"
)

// decoratedCreds builds a NATS decorated user creds file (the same format
// emitted by credentials.Credential.String) around the given JWT and seed.
func decoratedCreds(jwt, seed string) string {
	return fmt.Sprintf(`-----BEGIN NATS USER JWT-----
%s
------END NATS USER JWT------

-----BEGIN USER NKEY SEED-----
%s
------END USER NKEY SEED------
`, jwt, seed)
}

// TestNatsConnectionOptions verifies that the node's NATS connection options
// keep the client reconnecting through auth errors (e.g. an expired/rotated
// JWT) instead of permanently aborting the reconnect loop.
func TestNatsConnectionOptions(t *testing.T) {
	opts := natsConnectionOptions(&models.NatsConnectionData{
		ConnName:     "test",
		NatsUserJwt:  "jwt",
		NatsUserSeed: "seed",
	})

	// Each nats.Option is a func(*nats.Options) error; fold them into a fresh
	// Options struct to inspect the resulting configuration without opening a
	// real NATS connection.
	o := &nats.Options{}
	for _, opt := range opts {
		be.NilErr(t, opt(o))
	}

	be.True(t, o.IgnoreAuthErrorAbort)
	be.True(t, o.NoCallbacksAfterClientClose)
	be.Equal(t, -1, o.MaxReconnect)
}

// TestNatsConnectionOptions_CredsFile verifies that when NEX_NODE_NATS_CREDS_FILE
// points at a creds file, the built options authenticate via nats.UserCredentials
// (which re-reads the file on every reconnect so refreshed/re-minted creds are
// picked up automatically) instead of the static in-memory JWT+seed.
func TestNatsConnectionOptions_CredsFile(t *testing.T) {
	kp, err := nkeys.CreateUser()
	be.NilErr(t, err)
	seed, err := kp.Seed()
	be.NilErr(t, err)

	// A JWT-shaped token; nats.go extracts it verbatim from the decorated file.
	const fileJWT = "eyJ0eXAiOiJKV1QiLCJhbGciOiJlZDI1NTE5LW5rZXkifQ.eyJqdGkiOiJURVNUIn0.filesig"

	dir := t.TempDir()
	path := filepath.Join(dir, "node.creds")
	be.NilErr(t, os.WriteFile(path, []byte(decoratedCreds(fileJWT, string(seed))), 0o600))

	t.Setenv("NEX_NODE_NATS_CREDS_FILE", path)

	// Static creds are also present; the file must take precedence.
	opts := natsConnectionOptions(&models.NatsConnectionData{
		ConnName:     "test",
		NatsUserJwt:  "static-jwt",
		NatsUserSeed: string(seed),
	})

	o := &nats.Options{}
	for _, opt := range opts {
		be.NilErr(t, opt(o))
	}

	be.True(t, o.UserJWT != nil)
	be.True(t, o.SignatureCB != nil)

	// The JWT callback reads from the file, not the static "static-jwt" value.
	got, err := o.UserJWT()
	be.NilErr(t, err)
	be.Equal(t, fileJWT, got)
}
