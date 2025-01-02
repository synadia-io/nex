package actors

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/synadia-io/nex/models"
	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"

	"github.com/tochemey/goakt/v2/testkit"
)

func TestAgentSupervisor(t *testing.T) {
	ctx := context.Background()
	tk := testkit.New(ctx, t)

	as := &AgentSupervisor{
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		nodeOptions: models.NodeOptions{},
	}

	t.Cleanup(func() {
		tk.Shutdown(ctx)
	})

	t.Run("Send QueryWorkloads Message", func(t *testing.T) {
		tk.Spawn(ctx, AgentSupervisorActorName, as)
		probe := tk.NewProbe(ctx)
		msg := new(actorproto.QueryWorkloads)
		probe.SendSync(AgentSupervisorActorName, msg, time.Second)
		resp := &actorproto.WorkloadList{
			Workloads: []*actorproto.WorkloadSummary{},
		}
		probe.ExpectMessage(resp)
		probe.ExpectNoMessage()
		probe.Stop()
	})

	t.Run("Send SetLameDuck Message", func(t *testing.T) {
		tk.Spawn(ctx, AgentSupervisorActorName, as)
		probe := tk.NewProbe(ctx)
		msg := new(actorproto.SetLameDuck)
		probe.Send(AgentSupervisorActorName, msg)
		probe.ExpectNoMessage()
		probe.Stop()
	})
}
