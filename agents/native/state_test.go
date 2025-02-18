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
	"github.com/synadia-io/nexlet.go/agent"
	"github.com/synadia-labs/nex/models"
)

func MockRunner(t testing.TB) (*agent.Runner, error) {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	return agent.NewRunner("fake", "0.0.0", nil, agent.WithLogger(logger))
}

func TestNexletState(t *testing.T) {
	mockRunner, err := MockRunner(t)
	be.NilErr(t, err)

	ns := nexletState{
		Mutex:     sync.Mutex{},
		ctx:       context.Background(),
		runner:    mockRunner,
		status:    "starting",
		workloads: map[string]NativeProcesses{},
	}

	sleepPath, err := exec.LookPath("sleep")
	be.NilErr(t, err)

	asr := models.AgentStartWorkloadRequest{
		Request: models.StartWorkloadRequest{
			Description:       "sleeper",
			Name:              "sleeper",
			Namespace:         "derp",
			RunRequest:        fmt.Sprintf(`{"uri":"%s","args":["10"]}`, sleepPath),
			WorkloadLifecycle: "service",
			WorkloadType:      "native",
		},
		WorkloadCreds: models.NatsConnectionData{
			NatsUrl: "nats://127.0.0.1:4222",
		},
	}
	err = ns.AddWorkload("derp", "abc123", &asr)
	be.NilErr(t, err)

	be.Equal(t, 1, ns.NamespaceCount())
	be.Equal(t, 1, ns.WorkloadCount())
	sr, ok := ns.Exists("abc123")
	be.True(t, ok)
	be.DeepEqual(t, asr.Request, *sr)
	_, ok = ns.Exists("noexist")
	be.False(t, ok)

	list, err := ns.GetNamespaceWorkloadList("derp")
	be.NilErr(t, err)
	be.Equal(t, 1, len(*list))

	wls := (*list)[0]
	be.Nonzero(t, wls)
	be.Equal(t, "abc123", wls.Id)
	be.Equal(t, "sleeper", wls.Name)

	err = ns.RemoveWorkload("derp", "abc123")
	be.NilErr(t, err)

	time.Sleep(300 * time.Millisecond)

	be.Equal(t, 1, ns.NamespaceCount())
	be.Equal(t, 0, ns.WorkloadCount())
	_, ok = ns.Exists("abc123")
	be.False(t, ok)

	err = ns.AddWorkload("derp", "xyz890", &asr)
	be.NilErr(t, err)
	be.Equal(t, 1, ns.NamespaceCount())
	be.Equal(t, 1, ns.WorkloadCount())

	err = ns.SetLameduckMode(time.Second)
	be.NilErr(t, err)

	time.Sleep(2000 * time.Millisecond)
	be.Equal(t, 0, ns.NamespaceCount())
	be.Equal(t, 0, ns.WorkloadCount())
}
