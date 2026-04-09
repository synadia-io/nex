package native

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/sdk/go/agent"
	"github.com/synadia-io/nex/_test"
	"github.com/synadia-io/nex/models"
)

func waitFor(t testing.TB, timeout time.Duration, condition func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting: %s", msg)
}

func MockRunner(t testing.TB) (*agent.Runner, error) {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	return agent.NewRunner(context.TODO(), "nexus", "", nil, agent.WithLogger(logger))
}

func TestNexletState(t *testing.T) {
	workingDir := t.TempDir()
	binPath, _, _ := createTestBinary(t, workingDir)

	s := _test.StartNatsServer(t, workingDir)
	defer s.Shutdown()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)

	objArt, err := prepNatsObjStoreArtifact(t, nc, workingDir, binPath)
	be.NilErr(t, err)

	mockRunner, err := MockRunner(t)
	be.NilErr(t, err)

	ns := nexletState{
		Mutex:     sync.Mutex{},
		ctx:       context.Background(),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		runner:    mockRunner,
		status:    "starting",
		workloads: map[string]NativeProcesses{},
	}

	sleepPath, err := exec.LookPath("sleep")
	be.NilErr(t, err)

	tests := []struct {
		name     string
		workload models.AgentStartWorkloadRequest
	}{
		{
			name: "from_file",
			workload: models.AgentStartWorkloadRequest{
				Request: models.StartWorkloadRequest{
					Description:       "sleeper",
					Name:              "sleeper",
					Namespace:         "derp",
					RunRequest:        fmt.Sprintf(`{"uri":"%s","argv":["10"]}`, fmt.Sprintf("file://%s", sleepPath)),
					WorkloadLifecycle: "service",
					WorkloadType:      "native",
				},
				WorkloadCreds: models.NatsConnectionData{
					NatsServers: []string{s.ClientURL()},
				},
			},
		},
		{
			name: "from_nats",
			workload: models.AgentStartWorkloadRequest{
				Request: models.StartWorkloadRequest{
					Description:       "counter",
					Name:              "counter",
					Namespace:         "derp",
					RunRequest:        fmt.Sprintf(`{"uri":"%s"}`, objArt),
					WorkloadLifecycle: "service",
					WorkloadType:      "native",
				},
				WorkloadCreds: models.NatsConnectionData{
					NatsServers: []string{s.ClientURL()},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err = ns.AddWorkload("derp", "abc123", &tt.workload)
			be.NilErr(t, err)

			be.Equal(t, 1, ns.NamespaceCount())
			be.Equal(t, 1, ns.WorkloadCount())
			sr, ok := ns.Exists("abc123")
			be.True(t, ok)
			be.DeepEqual(t, tt.workload.Request, *sr)
			_, ok = ns.Exists("noexist")
			be.False(t, ok)

			list, err := ns.GetNamespaceWorkloadList("derp", nil)
			be.NilErr(t, err)
			be.Equal(t, 1, len(*list))

			wls := (*list)[0]
			be.Nonzero(t, wls)
			be.Equal(t, "abc123", wls.Id)
			be.Equal(t, tt.workload.Request.Name, wls.Name)

			err = ns.RemoveWorkload("derp", "abc123")
			be.NilErr(t, err)

			waitFor(t, 5*time.Second, func() bool {
				return ns.WorkloadCount() == 0
			}, "waiting for workload removal")

			be.Equal(t, 1, ns.NamespaceCount())
			be.Equal(t, 0, ns.WorkloadCount())
			_, ok = ns.Exists("abc123")
			be.False(t, ok)

			err = ns.AddWorkload("derp", "xyz890", &tt.workload)
			be.NilErr(t, err)
			be.Equal(t, 1, ns.NamespaceCount())
			be.Equal(t, 1, ns.WorkloadCount())

			err = ns.SetLameduckMode(time.Second)
			be.NilErr(t, err)

			waitFor(t, 10*time.Second, func() bool {
				return ns.NamespaceCount() == 0 && ns.WorkloadCount() == 0
			}, "waiting for lameduck cleanup")
			be.Equal(t, 0, ns.NamespaceCount())
			be.Equal(t, 0, ns.WorkloadCount())
		})
	}
}

func TestAddWorkloadWithInvalidUri(t *testing.T) {
	mockRunner, err := MockRunner(t)
	be.NilErr(t, err)

	ns := nexletState{
		Mutex:     sync.Mutex{},
		ctx:       context.Background(),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		runner:    mockRunner,
		status:    models.AgentStateStarting,
		workloads: map[string]NativeProcesses{},
	}

	const (
		namespace  = "test"
		workloadID = "12345"
	)

	tempDir := t.TempDir()
	req := models.AgentStartWorkloadRequest{
		Request: models.StartWorkloadRequest{
			Name:              "invalid_workload",
			Namespace:         namespace,
			RunRequest:        fmt.Sprintf(`{"uri":"file://%s"}`, tempDir),
			WorkloadLifecycle: "service",
			WorkloadType:      "native",
		},
	}

	err = ns.AddWorkload(req.Request.Namespace, workloadID, &req)
	if err == nil {
		t.Fatal("expected error when workload run request URI lacks file:// prefix")
	}

	be.Equal(t, 0, ns.WorkloadCount())

	_, ok := ns.Exists(workloadID)
	be.False(t, ok)
}

// TestGetNamespaceWorkloadListSystemAndFilter exercises GetNamespaceWorkloadList
// without spawning real processes. It verifies three things:
//   1. The system namespace is administrative and returns workloads across
//      every stored namespace, while a user namespace returns only its own.
//   2. The Namespace field on each WorkloadSummary is populated with the
//      workload's actual owning namespace (sourced from the state map key).
//   3. The filter argument matches against both workload id and workload
//      name; an empty filter returns everything.
func TestGetNamespaceWorkloadListSystemAndFilter(t *testing.T) {
	ns := nexletState{
		Mutex:     sync.Mutex{},
		ctx:       context.Background(),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		status:    models.AgentStateRunning,
		workloads: map[string]NativeProcesses{},
	}

	// Seed three workloads across two user namespaces directly via the
	// state map. AddWorkload spawns a real process, which is unnecessary
	// for a pure listing test.
	seed := func(namespace, id, name string) {
		if _, ok := ns.workloads[namespace]; !ok {
			ns.workloads[namespace] = make(NativeProcesses)
		}
		ns.workloads[namespace][id] = &NativeProcess{
			Name: name,
			StartRequest: models.StartWorkloadRequest{
				Name:              name,
				Namespace:         namespace,
				WorkloadLifecycle: "service",
			},
			StartedAt: time.Now(),
			State:     models.WorkloadStateRunning,
		}
	}
	seed("alpha", "id-a1", "alpha-one")
	seed("alpha", "id-a2", "alpha-two")
	seed("beta", "id-b1", "beta-one")

	namespaceOf := func(ws models.WorkloadSummary) string {
		t.Helper()
		if ws.Namespace == nil {
			t.Fatalf("workload %q is missing Namespace field", ws.Id)
		}
		return *ws.Namespace
	}

	t.Run("user namespace returns only its own workloads", func(t *testing.T) {
		list, err := ns.GetNamespaceWorkloadList("alpha", nil)
		be.NilErr(t, err)
		be.Equal(t, 2, len(*list))
		for _, ws := range *list {
			be.Equal(t, "alpha", namespaceOf(ws))
		}
	})

	t.Run("unknown namespace returns empty", func(t *testing.T) {
		list, err := ns.GetNamespaceWorkloadList("gamma", nil)
		be.NilErr(t, err)
		be.Equal(t, 0, len(*list))
	})

	t.Run("system namespace returns workloads from every namespace", func(t *testing.T) {
		list, err := ns.GetNamespaceWorkloadList(models.SystemNamespace, nil)
		be.NilErr(t, err)
		be.Equal(t, 3, len(*list))

		seen := map[string]string{}
		for _, ws := range *list {
			seen[ws.Id] = namespaceOf(ws)
		}
		be.Equal(t, "alpha", seen["id-a1"])
		be.Equal(t, "alpha", seen["id-a2"])
		be.Equal(t, "beta", seen["id-b1"])
	})

	t.Run("filter by id matches across system list", func(t *testing.T) {
		list, err := ns.GetNamespaceWorkloadList(models.SystemNamespace, []string{"id-a2"})
		be.NilErr(t, err)
		be.Equal(t, 1, len(*list))
		be.Equal(t, "id-a2", (*list)[0].Id)
		be.Equal(t, "alpha", namespaceOf((*list)[0]))
	})

	t.Run("filter by name matches across system list", func(t *testing.T) {
		list, err := ns.GetNamespaceWorkloadList(models.SystemNamespace, []string{"beta-one"})
		be.NilErr(t, err)
		be.Equal(t, 1, len(*list))
		be.Equal(t, "id-b1", (*list)[0].Id)
		be.Equal(t, "beta", namespaceOf((*list)[0]))
	})

	t.Run("filter mixes id and name and is scoped by namespace", func(t *testing.T) {
		list, err := ns.GetNamespaceWorkloadList("alpha", []string{"id-a1", "alpha-two", "beta-one"})
		be.NilErr(t, err)
		// beta-one is filtered out because the query is scoped to "alpha",
		// not system; id-a1 matches by id, alpha-two matches by name.
		be.Equal(t, 2, len(*list))
		ids := map[string]bool{}
		for _, ws := range *list {
			ids[ws.Id] = true
			be.Equal(t, "alpha", namespaceOf(ws))
		}
		be.True(t, ids["id-a1"])
		be.True(t, ids["id-a2"])
	})
}
