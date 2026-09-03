package nex

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/models"
)

// TestNodeStateConcurrentAccess drives REAL concurrent access to
// NexNode.nodeState and must be run under `go test -race`.
//
// Reader side: several goroutines fire node PING control requests. Each request
// the node serves runs handlePing, which reads nodeState (handlers.go, the
// NodePingResponse.State field) on a micro-service goroutine.
//
// Writer side: one goroutine drives a real state transition in a loop via
// enterLameduck, which writes nodeState (node.go).
//
// Before the nodeStateMu fix these unsynchronized reads and writes overlap and
// the race detector reports "WARNING: DATA RACE" on NexNode.nodeState. After the
// fix (getNodeState/setNodeState guard every access) the test passes clean.
//
// The writer runs for exactly as long as the readers are alive, so the read and
// write windows fully overlap regardless of scheduling. enterLameduck spawns a
// delayed-shutdown goroutine; a one-hour delay keeps it from firing during the
// test (the process exits long before).
func TestNodeStateConcurrentAccess(t *testing.T) {
	s := startNatsServer(t)
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	kp, err := nkeys.CreateServer()
	be.NilErr(t, err)

	pub, err := kp.PublicKey()
	be.NilErr(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	nn, err := NewNexNode(
		WithNatsConn(nc),
		WithLogger(logger),
		WithNodeKeyPair(kp),
	)
	be.NilErr(t, err)

	be.NilErr(t, nn.Start())
	be.NilErr(t, nn.IsReady(10*time.Second))

	// Fire the PING storm over an independent connection so it is not affected
	// by anything the node does to its own connection.
	reqConn, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer reqConn.Close()

	// DirectPing routes to handlePing on this specific node. The "filter" field
	// is required by NodePingRequest.UnmarshalJSON; an empty filter passes the
	// handler's filter check so it reaches the nodeState read.
	pingSubject := models.DirectPingRequestSubject(models.SystemNamespace, pub)
	pingData := []byte(`{"filter":{}}`)

	const readers = 8
	const readsPerReader = 200

	var readersWg sync.WaitGroup
	for i := 0; i < readers; i++ {
		readersWg.Add(1)
		go func() {
			defer readersWg.Done()
			for j := 0; j < readsPerReader; j++ {
				// Errors are intentionally ignored: the point of the test is
				// that the handler reads nodeState concurrently with the writer,
				// not that every request succeeds.
				_, _ = reqConn.Request(pingSubject, pingData, 2*time.Second)
			}
		}()
	}

	readersDone := make(chan struct{})
	go func() {
		readersWg.Wait()
		close(readersDone)
	}()

	// Writer: keep driving a real Running->Lameduck transition until the readers
	// finish, throttled so the write window spans the whole read window.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-readersDone:
				return
			default:
				nn.enterLameduck(time.Hour)
				time.Sleep(time.Millisecond)
			}
		}
	}()

	<-writerDone

	be.NilErr(t, nn.Shutdown())
}
