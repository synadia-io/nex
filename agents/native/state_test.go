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

			list, err := ns.GetNamespaceWorkloadList("derp")
			be.NilErr(t, err)
			be.Equal(t, 1, len(*list))

			wls := (*list)[0]
			be.Nonzero(t, wls)
			be.Equal(t, "abc123", wls.Id)
			be.Equal(t, tt.workload.Request.Name, wls.Name)

			err = ns.RemoveWorkload("derp", "abc123")
			be.NilErr(t, err)

			time.Sleep(300 * time.Millisecond)

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

			time.Sleep(2000 * time.Millisecond)
			be.Equal(t, 0, ns.NamespaceCount())
			be.Equal(t, 0, ns.WorkloadCount())
		})
	}
}
