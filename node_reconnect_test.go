package nex

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/models"
)

// startNatsServerOnPort starts an embedded nats-server bound to an explicit
// host:port with a caller-owned JetStream store dir, so the server can be shut
// down and brought back up on the SAME client URL. Passing port -1 lets the OS
// pick; capture s.Addr() to restart on the same port afterwards.
func startNatsServerOnPort(t testing.TB, host string, port int, storeDir string) *server.Server {
	t.Helper()
	s, err := server.NewServer(&server.Options{
		Host:      host,
		Port:      port,
		JetStream: true,
		StoreDir:  storeDir,
	})
	be.NilErr(t, err)
	s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats server failed to start")
	}
	return s
}

// TestNodeControlServiceSurvivesNatsReconnect drives a REAL disconnect->reconnect
// end to end: it builds the node's NATS connection through the production
// configureNatsConnection path (MaxReconnects(-1)), starts the node against a
// real embedded nats-server, then stops that server and restarts it on the SAME
// host:port so the node's client reconnects on its own. It asserts a control
// PING (handlePing) succeeds both before AND after the reconnect -- i.e. the node
// stays reachable across a NATS blip.
//
// SCOPE / what this does and does NOT prove (verified against nats.go v1.49.0):
//
//   - It exercises configureNatsConnection + natsConnectionOptions (the real
//     connect path, otherwise at 0% coverage) and handlePing before/after a
//     reconnect.
//   - It does NOT by itself exercise the control-service rebuild fix
//     (node.go rebuildServiceForever / onServiceStopped DoneHandler). A plain
//     disconnect->reconnect does NOT stop the micro service: with
//     MaxReconnects(-1) the connection never closes and nats.go transparently
//     re-establishes every subscription on reconnect, so micro never fires its
//     DoneHandler. This was verified empirically (8s outage + repeated flaps:
//     service.Stopped() stays false throughout, control keeps serving).
//     nats.go micro stops a service only on a connection CLOSE (from which the
//     rebuild loop bails because n.nc.IsClosed()) or on an async endpoint error
//     (e.g. a slow consumer). The rebuild logic itself is covered by
//     TestNodeControlServiceRebuildsAfterUnexpectedStop and
//     TestNodeBuildServiceRefusesCommitDuringShutdown, which drive micro Stop()
//     directly.
//
// So this test guards "the node survives a NATS reconnect and control still
// answers", not the rebuild-after-unexpected-stop path.
func TestNodeControlServiceSurvivesNatsReconnect(t *testing.T) {
	storeDir := t.TempDir()
	s := startNatsServerOnPort(t, "127.0.0.1", -1, storeDir)
	// Capture the OS-chosen port so we can restart on the same client URL.
	port := s.Addr().(*net.TCPAddr).Port
	clientURL := s.ClientURL()
	// s is reassigned when the server is restarted below; serverDown tracks
	// whether the current s is already shut down so the defer doesn't double-stop.
	serverDown := false
	defer func() {
		if !serverDown {
			s.Shutdown()
		}
	}()

	// Build the node's connection via the real production path so the connection
	// carries MaxReconnects(-1) and reconnects on its own. Pointing NatsServers
	// at the test server also gives configureNatsConnection real coverage.
	nc, err := configureNatsConnection(&models.NatsConnectionData{
		NatsServers: []string{clientURL},
	})
	be.NilErr(t, err)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)
	pub, err := kp.PublicKey()
	be.NilErr(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nn, err := NewNexNode(WithNatsConn(nc), WithLogger(logger), WithNodeKeyPair(kp))
	be.NilErr(t, err)
	be.NilErr(t, nn.Start())
	be.NilErr(t, nn.IsReady(10*time.Second))

	// A control PING goes through the micro service -> handlePing. Request
	// subject $NEX.SVC.system.control.PING.<node-id> maps to the "PingNode"
	// endpoint.
	pingSubj := models.DirectPingRequestSubject(models.SystemNamespace, pub)
	pingBody, err := json.Marshal(models.NodePingRequest{Filter: models.NodePingRequestFilter{}})
	be.NilErr(t, err)

	// Serves before the reconnect.
	pre, err := nc.Request(pingSubj, pingBody, time.Second)
	be.NilErr(t, err)
	preResp := models.NodePingResponse{}
	be.NilErr(t, json.Unmarshal(pre.Data, &preResp))
	be.Equal(t, pub, preResp.NodeId)

	// Force a real disconnect: stop the server and wait for the client to notice.
	s.Shutdown()
	s.WaitForShutdown()
	serverDown = true
	disconnected := false
	for i := 0; i < 100; i++ {
		if !nc.IsConnected() {
			disconnected = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	be.True(t, disconnected) // the node's client actually saw the outage

	// Bring the server back on the SAME host:port so the existing client
	// reconnects (MaxReconnects(-1)); do NOT create a new client.
	s = startNatsServerOnPort(t, "127.0.0.1", port, storeDir)
	serverDown = false

	// Wait for the node's client to reconnect on its own.
	reconnected := false
	for i := 0; i < 200; i++ {
		if nc.IsConnected() {
			reconnected = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	be.True(t, reconnected)

	// Control must answer AGAIN after the reconnect. Poll with a bounded retry
	// loop: reconnection and subscription re-establishment are not instant.
	var postResp models.NodePingResponse
	served := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		msg, rerr := nc.Request(pingSubj, pingBody, 300*time.Millisecond)
		if rerr == nil {
			if uerr := json.Unmarshal(msg.Data, &postResp); uerr == nil {
				served = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	be.True(t, served)
	be.Equal(t, pub, postResp.NodeId)

	be.NilErr(t, nn.Shutdown())
}
