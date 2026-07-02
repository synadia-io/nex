package nex

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"

	"github.com/synadia-io/nex/internal/credentials"
	"github.com/synadia-io/nex/models"
)

// ---------------------------------------------------------------------------
// LAYER 1: deterministic auth-error reconnect behavior.
//
// A raw TCP mock server speaks just enough of the NATS protocol to (a) let the
// initial Connect() succeed, then (b) return a repeated auth error on every
// reconnect attempt. This isolates the reconnect/abort logic from JWT timing so
// the test is fast and deterministic (blueprint: nats.go TestExpiredAuthentication).
//
//   - Control (PRE-fix options, no IgnoreAuthErrorAbort): nats.go aborts the
//     reconnect loop after two identical auth errors and permanently CLOSES the
//     connection.
//   - Fixed (production natsConnectionOptions, which sets IgnoreAuthErrorAbort +
//     MaxReconnects(-1)): the client keeps reconnecting forever and never closes.
// ---------------------------------------------------------------------------

// startAuthErrMockServer accepts NATS clients and, after the first (successful)
// connect, replies to every reconnect handshake with an auth error. It returns
// the listen address, a live counter of accepted connections, and a stop func.
func startAuthErrMockServer(t *testing.T) (addr string, accepted *int32, stop func()) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	be.NilErr(t, err)

	var count int32
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			conn, err := l.Accept()
			if err != nil {
				return // listener closed
			}
			n := atomic.AddInt32(&count, 1)
			go func(conn net.Conn, n int32) {
				defer conn.Close()

				// Advertise ourselves so the client proceeds with the handshake.
				_, _ = conn.Write([]byte(`INFO {"server_id":"mock","nonce":"abc"}` + "\r\n"))

				// The client sends CONNECT then PING during (re)connect.
				br := bufio.NewReaderSize(conn, 10*1024)
				_, _, _ = br.ReadLine()
				_, _, _ = br.ReadLine()

				if n == 1 {
					// First connect succeeds, then we asynchronously report an
					// expired credential and drop the socket, forcing a reconnect.
					_, _ = conn.Write([]byte("PONG\r\n"))
					time.Sleep(100 * time.Millisecond)
					_, _ = conn.Write([]byte("-ERR 'user authentication expired'\r\n"))
					return
				}
				// Every reconnect handshake gets the same auth error.
				_, _ = conn.Write([]byte("-ERR 'Authorization Violation'\r\n"))
			}(conn, n)
		}
	}()

	return l.Addr().String(), &count, func() {
		_ = l.Close()
		wg.Wait()
	}
}

func TestNATSReconnectThroughAuthErrors(t *testing.T) {
	t.Run("prefix_options_abort_and_close", func(t *testing.T) {
		addr, accepted, stop := startAuthErrMockServer(t)
		defer stop()

		closed := make(chan struct{}, 1)

		// The PRE-fix option set: infinite reconnects but WITHOUT
		// IgnoreAuthErrorAbort (mirrors node.go before hotfix-010). Fast
		// reconnect timing keeps the test quick.
		opts := []nats.Option{
			nats.Name("nexnode"),
			nats.MaxReconnects(-1),
			nats.Timeout(10 * time.Second),
			nats.ReconnectWait(25 * time.Millisecond),
			nats.ReconnectJitter(0, 0),
			nats.ClosedHandler(func(_ *nats.Conn) {
				select {
				case closed <- struct{}{}:
				default:
				}
			}),
		}

		nc, err := nats.Connect("nats://"+addr, opts...)
		be.NilErr(t, err)
		defer nc.Close()

		// The connection must permanently close after two identical auth errors.
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			t.Fatal("pre-fix connection should close after repeated auth errors")
		}
		be.True(t, nc.IsClosed())

		// It gives up quickly: initial connect + a small number of aborted
		// reconnect attempts, not an unbounded loop.
		be.True(t, atomic.LoadInt32(accepted) <= 4)
	})

	t.Run("production_options_keep_reconnecting", func(t *testing.T) {
		addr, accepted, stop := startAuthErrMockServer(t)
		defer stop()

		// Exercise the REAL production helper. Keep the creds-file branch out of
		// play so we test the in-memory (no-creds) reconnect path here.
		t.Setenv("NEX_NODE_NATS_CREDS_FILE", "")

		closed := make(chan struct{}, 1)

		connData := &models.NatsConnectionData{NatsServers: []string{addr}}
		opts := natsConnectionOptions(connData)
		// Appended on top of the production option set: only tightens reconnect
		// timing and observes closure. IgnoreAuthErrorAbort / MaxReconnects(-1)
		// come from natsConnectionOptions and are untouched.
		opts = append(opts,
			nats.ReconnectWait(25*time.Millisecond),
			nats.ReconnectJitter(0, 0),
			nats.ClosedHandler(func(_ *nats.Conn) {
				select {
				case closed <- struct{}{}:
				default:
				}
			}),
		)

		nc, err := nats.Connect("nats://"+addr, opts...)
		be.NilErr(t, err)
		defer nc.Close()

		// The client must keep reconnecting through the auth errors: observe
		// well more than the 2-attempt pre-fix abort threshold.
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) && atomic.LoadInt32(accepted) < 5 {
			time.Sleep(25 * time.Millisecond)
		}
		be.True(t, atomic.LoadInt32(accepted) >= 5)

		// ...and it must NOT have closed.
		select {
		case <-closed:
			t.Fatal("production connection should not close on repeated auth errors")
		default:
		}
		be.False(t, nc.IsClosed())
	})
}

// ---------------------------------------------------------------------------
// LAYER 2: live creds-file rotation self-heal against a real operator-mode
// nats-server.
//
// Flow: mint a short-lived user JWT -> write a decorated creds file -> point
// NEX_NODE_NATS_CREDS_FILE at it -> connect via the production helper -> confirm
// a round-trip -> wait for the server to enforce JWT expiry and disconnect us ->
// mint FRESH creds, overwrite the SAME file -> confirm the connection self-heals
// and round-trips again, without rebuilding options.
//
// Disconnect mechanism: server-side JWT expiry enforcement. nats-server sets an
// expiration timer from the user JWT `exp` claim (setExpiration ->
// setExpirationTimer -> authExpired) and closes the connection with an
// "authentication expired" error when it fires. No manual restart/ForceReconnect
// is needed. The client's UserCredentials option re-reads the creds file on every
// reconnect, so overwriting the file with fresh creds lets the connection recover.
// ---------------------------------------------------------------------------

type operatorEnv struct {
	url       string
	accountKP nkeys.KeyPair
}

// startOperatorNatsServer boots an embedded nats-server in operator/JWT trust
// mode: a fresh operator signs a fresh account JWT held in an in-memory account
// resolver. User JWTs are signed by the account key.
func startOperatorNatsServer(t *testing.T) *operatorEnv {
	t.Helper()

	okp, err := nkeys.CreateOperator()
	be.NilErr(t, err)
	opub, err := okp.PublicKey()
	be.NilErr(t, err)

	akp, err := nkeys.CreateAccount()
	be.NilErr(t, err)
	apub, err := akp.PublicKey()
	be.NilErr(t, err)

	ac := jwt.NewAccountClaims(apub)
	ac.Name = "NEX_TEST"
	ajwt, err := ac.Encode(okp)
	be.NilErr(t, err)

	resolver := &server.MemAccResolver{}
	be.NilErr(t, resolver.Store(apub, ajwt))

	s, err := server.NewServer(&server.Options{
		Host:        "127.0.0.1",
		Port:        -1,
		TrustedKeys: []string{opub},
	})
	be.NilErr(t, err)
	s.SetAccountResolver(resolver)

	s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("operator nats server failed to start")
	}
	t.Cleanup(s.Shutdown)

	return &operatorEnv{url: s.ClientURL(), accountKP: akp}
}

// mintUserCreds builds a real user credential signed by the account key with an
// explicit (short) expiry. The minter in internal/credentials hardcodes a ~1y
// TTL, so we construct the claims directly with the same libraries and reuse
// credentials.Credential for the decorated creds-file encoding.
func mintUserCreds(t *testing.T, accountKP nkeys.KeyPair, ttl time.Duration) *credentials.Credential {
	t.Helper()

	ukp, err := nkeys.CreateUser()
	be.NilErr(t, err)
	upub, err := ukp.PublicKey()
	be.NilErr(t, err)

	claims := jwt.NewUserClaims(upub)
	claims.Subject = upub
	claims.Expires = time.Now().Add(ttl).Unix()
	// Empty permissions => allow all pub/sub within the account, enough for a
	// round-trip.

	ujwt, err := claims.Encode(accountKP)
	be.NilErr(t, err)

	seed, err := ukp.Seed()
	be.NilErr(t, err)

	return &credentials.Credential{Jwt: ujwt, NkeySeed: seed}
}

// writeCredsFile atomically writes the decorated creds file so a reconnecting
// client never reads a half-written file.
func writeCredsFile(t *testing.T, path string, c *credentials.Credential) {
	t.Helper()
	tmp := path + ".tmp"
	be.NilErr(t, os.WriteFile(tmp, []byte(c.String()), 0o600))
	be.NilErr(t, os.Rename(tmp, path))
}

const selfHealEcho = "nex.selfheal.echo"

// roundTrip proves the connection is authorized and usable: a request to an
// in-connection responder must return the expected reply.
func roundTrip(nc *nats.Conn) error {
	resp, err := nc.Request(selfHealEcho, []byte("ping"), 1*time.Second)
	if err != nil {
		return err
	}
	if string(resp.Data) != "pong" {
		return fmt.Errorf("unexpected response %q", resp.Data)
	}
	return nil
}

func waitForRoundTrip(t *testing.T, nc *nats.Conn, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if nc.IsConnected() {
			if lastErr = roundTrip(nc); lastErr == nil {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("connection did not self-heal within %s (last round-trip err: %v)", timeout, lastErr)
}

func waitForSignal(t *testing.T, ch <-chan struct{}, timeout time.Duration, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatal(msg)
	}
}

func TestCredsFileRotationSelfHeal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live creds-rotation e2e test in -short mode")
	}

	env := startOperatorNatsServer(t)

	credsPath := filepath.Join(t.TempDir(), "node.creds")
	writeCredsFile(t, credsPath, mintUserCreds(t, env.accountKP, 3*time.Second))
	t.Setenv("NEX_NODE_NATS_CREDS_FILE", credsPath)

	disconnected := make(chan struct{}, 16)

	connData := &models.NatsConnectionData{NatsServers: []string{env.url}}
	opts := natsConnectionOptions(connData)
	// Observation/timing handlers appended on top of the production option set.
	// These override only the logging handlers; the reconnect semantics under
	// test (IgnoreAuthErrorAbort, MaxReconnects(-1), UserCredentials re-read of
	// the env creds file) come from natsConnectionOptions and are untouched.
	opts = append(opts,
		nats.ReconnectWait(200*time.Millisecond),
		nats.ReconnectJitter(0, 0),
		nats.DisconnectErrHandler(func(_ *nats.Conn, _ error) {
			select {
			case disconnected <- struct{}{}:
			default:
			}
		}),
	)

	nc, err := nats.Connect(env.url, opts...)
	be.NilErr(t, err)
	defer nc.Close()

	// In-connection responder for round-trip checks (same account, allow-all).
	_, err = nc.Subscribe(selfHealEcho, func(m *nats.Msg) { _ = m.Respond([]byte("pong")) })
	be.NilErr(t, err)
	be.NilErr(t, nc.Flush())

	// Healthy before expiry.
	be.NilErr(t, roundTrip(nc))

	// The server enforces the JWT expiry and kicks us.
	waitForSignal(t, disconnected, 15*time.Second, "expected disconnect after JWT expiry")

	// Crucially, the fix keeps the reconnect loop alive: the connection is not
	// permanently closed.
	be.False(t, nc.IsClosed())

	// Rotate: mint fresh long-lived creds and atomically overwrite the SAME
	// file. No connection options are rebuilt.
	writeCredsFile(t, credsPath, mintUserCreds(t, env.accountKP, time.Hour))

	// Self-heal: within a bounded window the connection recovers and a
	// round-trip succeeds again.
	waitForRoundTrip(t, nc, 15*time.Second)
	be.False(t, nc.IsClosed())
}

func TestCredsFileNoRotationKeepsRetrying(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live creds-rotation e2e test in -short mode")
	}

	env := startOperatorNatsServer(t)

	credsPath := filepath.Join(t.TempDir(), "node.creds")
	writeCredsFile(t, credsPath, mintUserCreds(t, env.accountKP, 3*time.Second))
	t.Setenv("NEX_NODE_NATS_CREDS_FILE", credsPath)

	disconnected := make(chan struct{}, 64)
	var retryEvents int32

	connData := &models.NatsConnectionData{NatsServers: []string{env.url}}
	opts := natsConnectionOptions(connData)
	opts = append(opts,
		nats.ReconnectWait(200*time.Millisecond),
		nats.ReconnectJitter(0, 0),
		nats.DisconnectErrHandler(func(_ *nats.Conn, _ error) {
			atomic.AddInt32(&retryEvents, 1)
			select {
			case disconnected <- struct{}{}:
			default:
			}
		}),
		// Each failed reconnect handshake surfaces an auth error here; counting
		// them proves the reconnect loop stays alive instead of aborting.
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			if err != nil {
				atomic.AddInt32(&retryEvents, 1)
			}
		}),
	)

	nc, err := nats.Connect(env.url, opts...)
	be.NilErr(t, err)
	defer nc.Close()

	_, err = nc.Subscribe(selfHealEcho, func(m *nats.Msg) { _ = m.Respond([]byte("pong")) })
	be.NilErr(t, err)
	be.NilErr(t, nc.Flush())
	be.NilErr(t, roundTrip(nc))

	// Server enforces expiry and disconnects us.
	waitForSignal(t, disconnected, 15*time.Second, "expected disconnect after JWT expiry")

	// Without rotation the connection can never recover (the file still holds an
	// expired JWT), but the fix must keep it retrying rather than permanently
	// closing. Assert it never goes CLOSED across the window and that retry
	// activity keeps accumulating (pre-fix would have aborted after 2 attempts).
	//
	// We deliberately assert reconnect-loop liveness rather than "round-trip
	// fails", because an expired JWT can briefly connect-then-expire, which
	// would make a strict round-trip-failure assertion racy.
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		be.False(t, nc.IsClosed())
		time.Sleep(100 * time.Millisecond)
	}
	be.False(t, nc.IsClosed())
	be.True(t, atomic.LoadInt32(&retryEvents) >= 3)
}
