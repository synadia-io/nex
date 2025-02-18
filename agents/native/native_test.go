package native

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/synadia-labs/nex/_test"
	"github.com/synadia-labs/nex/models"
)

func TestNewNativeRunner(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	nn, err := NewNativeWorkloadRunner(context.Background(), _test.Node1Pub, logger)
	be.NilErr(t, err)
	be.Nonzero(t, nn)
}

func TestNewWorkloadAgent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	na, err := newNativeWorkloadAgent(context.Background(), _test.Node1Pub, logger)
	be.NilErr(t, err)
	be.Nonzero(t, na)

	t.Run("TestRegister", func(t *testing.T) {
		req, err := na.Register()
		be.NilErr(t, err)
		be.Equal(t, NEXLET_NAME, req.Name)
		be.Equal(t, VERSION, req.Version)
		be.Equal(t, "Runs workloads as subprocesses on the host machine", req.Description)
		be.Equal(t, 0, req.MaxWorkloads)
		be.Nonzero(t, req.PublicXkey)
		be.AllEqual(t, []models.WorkloadLifecycle{models.WorkloadLifecycleJob, models.WorkloadLifecycleService}, req.SupportedLifecycles)
		be.Equal(t, startRequest, req.StartRequestSchema)
	})

	t.Run("TestHeartbeat", func(t *testing.T) {
		req, err := na.Heartbeat()
		be.NilErr(t, err)

		stats := struct {
			TotalNamespaces int `json:"namespace_count"`
		}{
			TotalNamespaces: 0,
		}

		statsB, err := json.Marshal(stats)
		be.NilErr(t, err)

		be.Equal(t, string(statsB), req.Data)
		be.Equal(t, string(na.agentState), req.State)
	})
}
