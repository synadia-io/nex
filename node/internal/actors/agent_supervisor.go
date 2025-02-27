package actors

import (
	"context"
	"io"
	"log/slog"

	"github.com/tochemey/goakt/v3/goaktpb"

	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v3/actor"

	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
)

const AgentSupervisorActorName = "agent_supervisor"

var _ goakt.Actor = (*AgentSupervisor)(nil)

func CreateAgentSupervisor(options models.NodeOptions) *AgentSupervisor {
	return &AgentSupervisor{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), nodeOptions: options}
}

// Agent manager is responsible for starting one agent per workload type, supplying it with
// the right parameters, and keeping track of the information received from its initial
// registration
type AgentSupervisor struct {
	logger *slog.Logger

	nodeOptions models.NodeOptions
}

func (s *AgentSupervisor) PreStart(ctx context.Context) error {
	return nil
}

func (s *AgentSupervisor) PostStop(ctx context.Context) error {
	s.logger.Debug("Agent supervisor stopped")
	return nil
}

func (s *AgentSupervisor) Receive(ctx *goakt.ReceiveContext) {
	switch m := ctx.Message().(type) {
	case *goaktpb.PostStart:
		s.logger = s.nodeOptions.Logger.WithGroup(AgentSupervisorActorName)
	case *actorproto.QueryWorkloads:
		s.queryWorkloads(ctx)
	case *actorproto.SetLameDuck:
		s.logger.Debug("Received set lame duck message")
		s.setLameDuck(ctx)
	case *goaktpb.Terminated:
		s.logger.Debug("Received terminated message", slog.String("actor", m.ActorId))
	default:
		s.logger.Warn("Received unhandled message", slog.Any("msg", ctx.Message()), slog.String("actor", ctx.Self().ID()))
		ctx.Unhandled()
	}
}

// Ask each child of the supervisor for its list of running workloads and then return
// the aggregate result
func (s *AgentSupervisor) queryWorkloads(ctx *goakt.ReceiveContext) {
	ctxx := context.Background()
	workloads := make([]*actorproto.WorkloadSummary, 0)

	for _, pid := range ctx.Self().Children() {
		res, err := ctx.Self().Ask(ctxx, pid, &actorproto.QueryWorkloads{}, DefaultAskDuration)
		if err != nil {
			// TODO: todo
			continue
		}
		workloads = append(workloads, res.(*actorproto.WorkloadList).Workloads...)
	}
	ctx.Response(&actorproto.WorkloadList{Workloads: workloads})
}

func (s *AgentSupervisor) setLameDuck(ctx *goakt.ReceiveContext) {
	for _, pid := range ctx.Self().Children() {
		r, err := ctx.Self().Ask(context.Background(), pid, &actorproto.SetLameDuck{}, DefaultAskDuration)
		if err != nil {
			s.logger.Error("Child actor failed to set lame duck", slog.Any("err", err), slog.String("parent", ctx.Self().ID()), slog.String("child", pid.ID()))
			return
		}
		resp, ok := r.(*actorproto.LameDuckResponse)
		if !ok {
			s.logger.Warn("Unexpected response from agent")
			return
		}
		if !resp.Success {
			s.logger.Error("Child workload failed to shutdown", slog.String("child", pid.ID()), slog.Any("err", err))
			return
		}
	}
}
