package internal

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	eventemitter "github.com/synadia-io/nex/internal/event_emitter"
	"github.com/synadia-io/nex/internal/idgen"
	"github.com/synadia-io/nex/models"
)

const (
	nodeSeed = "SNAD2VTZQ7Z3CCB6TJWFFJK6J7DDZ5SQUHOFPQFRKDIVDQBM2WLGZGELSE"
	nodePub  = "NB4XOM2IOV2NRLRZZU5PFMQFBAQXYQOG4WOVGOWRYJRBWI3JO5QJK7AG"
)

func startNatsServer(t testing.TB, workDir string) *server.Server {
	t.Helper()

	server := server.New(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  workDir,
	})

	server.Start()
	if !server.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats server failed to start")
	}

	return server
}

func TestWatcherRestart(t *testing.T) {
	s := startNatsServer(t, t.TempDir())
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)

	stdout := new(bytes.Buffer)
	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	kp, err := nkeys.FromSeed([]byte(nodeSeed))
	be.NilErr(t, err)

	var wg sync.WaitGroup
	wg.Add(1) // Testing one agent with restart

	emitter := eventemitter.NewLogEmitter(t.Context(), logger, slog.LevelDebug)

	at := NewAgentWatcher(t.Context(), nc, kp, nodePub, nil, nil, logger, emitter, 1, &wg)
	uri, err := exec.LookPath("sleep")
	be.NilErr(t, err)

	ap := &AgentProcess{
		Config: &models.Agent{
			Uri:  uri,
			Argv: []string{"1"},
		},
		ID:           "abc",
		HostNode:     "node1",
		restartCount: 0,
		state:        "testing",
	}

	at.StartLocalBinaryAgent(ap)

	be.True(t, strings.Contains(stdout.String(), `level=INFO msg="started local agent" agent=sleep restart_count=0 reset_limit=1`))
	be.True(t, strings.Contains(stdout.String(), `level=INFO msg="started local agent" agent=sleep restart_count=1 reset_limit=1`))
	be.Equal(t, 2, strings.Count(stdout.String(), `level=WARN msg="Nexlet process unexpectedly exited with state" state="exit status 0"`))
}

// recordingRegisterMinter records the agent id passed to each MintRegister so
// a test can assert a fresh instance id is issued per (re)start.
type recordingRegisterMinter struct {
	mu          sync.Mutex
	registerIDs []string
	servers     []string
}

func (m *recordingRegisterMinter) MintRegister(agentID, nodeID string) (*models.NatsConnectionData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registerIDs = append(m.registerIDs, agentID)
	return &models.NatsConnectionData{NatsServers: m.servers}, nil
}

func (m *recordingRegisterMinter) Mint(_ models.CredType, _, _ string) (*models.NatsConnectionData, error) {
	return &models.NatsConnectionData{NatsServers: m.servers}, nil
}

func (m *recordingRegisterMinter) ids() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.registerIDs...)
}

// TestWatcherRestartMintsFreshInstanceID pins the fresh-identity half of the
// crash-recovery fix: every (re)start of a local agent mints register
// credentials under a NEW instance id, rather than reusing one across
// restarts. Reusing the id is what made a restarted agent collide with its own
// dead registration and be refused. With resetLimit=1 the process starts twice
// (initial + one restart), so there must be two mints under two distinct ids.
func TestWatcherRestartMintsFreshInstanceID(t *testing.T) {
	s := startNatsServer(t, t.TempDir())
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	kp, err := nkeys.FromSeed([]byte(nodeSeed))
	be.NilErr(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	emitter := eventemitter.NewLogEmitter(t.Context(), logger, slog.LevelDebug)
	minter := &recordingRegisterMinter{servers: []string{s.ClientURL()}}

	at := NewAgentWatcher(t.Context(), nc, kp, nodePub, minter, idgen.NewNuidGen(), logger, emitter, 1, &wg)
	uri, err := exec.LookPath("sleep")
	be.NilErr(t, err)

	ap := &AgentProcess{
		Config:       &models.Agent{Uri: uri, Argv: []string{"1"}},
		ID:           "slot-abc",
		HostNode:     "node1",
		restartCount: 0,
		state:        "testing",
	}

	at.StartLocalBinaryAgent(ap)

	ids := minter.ids()
	be.Equal(t, 2, len(ids))
	be.Unequal(t, ids[0], ids[1])
	// The stable slot id is not what gets registered -- each instance is new.
	be.Unequal(t, "slot-abc", ids[0])
}

func TestWatcherNewAgentBadCommand(t *testing.T) {
	s := startNatsServer(t, t.TempDir())
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)

	stdout := new(bytes.Buffer)
	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	kp, err := nkeys.FromSeed([]byte(nodeSeed))
	be.NilErr(t, err)

	var wg sync.WaitGroup
	wg.Add(1) // Testing one agent with restart

	emitter := eventemitter.NewLogEmitter(t.Context(), logger, slog.LevelDebug)

	at := NewAgentWatcher(t.Context(), nc, kp, nodePub, nil, nil, logger, emitter, 1, &wg)

	ap := &AgentProcess{
		Config: &models.Agent{
			Uri: "foobar",
		},
		ID:           "abc",
		HostNode:     "node1",
		restartCount: 0,
		state:        "testing",
	}
	at.StartLocalBinaryAgent(ap)
	be.True(t, strings.Contains(stdout.String(), `level=ERROR msg="provide path is not a binary file" agent_uri=foobar err="stat foobar: no such file or directory"`))
}

func TestWatcherNewAgentBadBinary(t *testing.T) {
	s := startNatsServer(t, t.TempDir())
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)

	stdout := new(bytes.Buffer)
	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	kp, err := nkeys.FromSeed([]byte(nodeSeed))
	be.NilErr(t, err)

	var wg sync.WaitGroup
	wg.Add(1) // Testing one agent with restart

	emitter := eventemitter.NewLogEmitter(t.Context(), logger, slog.LevelDebug)
	at := NewAgentWatcher(t.Context(), nc, kp, nodePub, nil, nil, logger, emitter, 1, &wg)

	fakeBinary, err := os.CreateTemp(t.TempDir(), "fakebin*")
	be.NilErr(t, err)
	err = os.Chmod(fakeBinary.Name(), 0755)
	be.NilErr(t, err)
	_ = fakeBinary.Close()

	ap := &AgentProcess{
		Config: &models.Agent{
			Uri: fakeBinary.Name(),
		},
		ID:           "abc",
		HostNode:     "node1",
		restartCount: 0,
		state:        "testing",
	}
	at.StartLocalBinaryAgent(ap)
	be.Equal(t, 2, strings.Count(stdout.String(), `level=ERROR msg="failed to start local agent"`))
}

func TestWatcherNewAgentBadCommandArgs(t *testing.T) {
	s := startNatsServer(t, t.TempDir())
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)

	stdout := new(bytes.Buffer)
	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	kp, err := nkeys.FromSeed([]byte(nodeSeed))
	be.NilErr(t, err)

	var wg sync.WaitGroup
	wg.Add(1) // Testing one agent with restart

	emitter := eventemitter.NewLogEmitter(t.Context(), logger, slog.LevelDebug)

	at := NewAgentWatcher(t.Context(), nc, kp, nodePub, nil, nil, logger, emitter, 1, &wg)

	uri, err := exec.LookPath("sleep")
	be.NilErr(t, err)

	ap := &AgentProcess{
		Config: &models.Agent{
			Uri: uri,
		},
		ID:           "abc",
		HostNode:     "node1",
		restartCount: 0,
		state:        "testing",
	}

	at.StartLocalBinaryAgent(ap)
	be.True(t, strings.Contains(stdout.String(), `level=INFO msg="started local agent" agent=sleep restart_count=0 reset_limit=1`))
	be.True(t, strings.Contains(stdout.String(), `level=INFO msg="started local agent" agent=sleep restart_count=1 reset_limit=1`))

	// For whatever reason, these checks add flake to the test suite on osx
	// be.Equal(t, 2, strings.Count(stdout.String(), `level=ERROR`)) // error message is different for linux/osx so the CI breaks if we include any more context
	// be.Equal(t, 2, strings.Count(stdout.String(), `level=WARN msg="Nexlet process unexpectedly exited with state" state="exit status 1"`))
}
