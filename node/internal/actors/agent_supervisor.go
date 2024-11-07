package actors

import (
	"context"
	"log/slog"

	"github.com/tochemey/goakt/v2/goaktpb"

	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v2/actors"

	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
)

const AgentSupervisorActorName = "agent_supervisor"

var _ goakt.Actor = (*AgentSupervisor)(nil)

func CreateAgentSupervisor(system goakt.ActorSystem, options models.NodeOptions) *AgentSupervisor {
	return &AgentSupervisor{nodeOptions: options}
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
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		s.logger = s.nodeOptions.Logger.WithGroup(AgentSupervisorActorName)
	case *actorproto.QueryWorkloads:
		s.queryWorkloads(ctx)
	case *actorproto.SetLameDuck:
		s.logger.Debug("Received set lame duck message")
		ctx.Response(s.setLameDuck(ctx))
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
		res, err := ctx.Self().Ask(ctxx, pid, &actorproto.QueryWorkloads{})
		if err != nil {
			// TODO: todo
			continue
		}
		workloads = append(workloads, res.(*actorproto.WorkloadList).Workloads...)
	}
	ctx.Response(&actorproto.WorkloadList{Workloads: workloads})
}

func (s *AgentSupervisor) setLameDuck(ctx *goakt.ReceiveContext) *actorproto.LameDuckResponse {
	for _, pid := range ctx.Self().Children() {
		r, err := ctx.Self().Ask(context.Background(), pid, &actorproto.SetLameDuck{})
		if err != nil {
			s.logger.Error("Child actor failed to set lame duck", slog.Any("err", err), slog.String("parent", ctx.Self().ID()), slog.String("child", pid.ID()))
			return &actorproto.LameDuckResponse{Id: ctx.Self().ID(), Success: false}
		}
		resp, ok := r.(*actorproto.LameDuckResponse)
		if !ok {
			s.logger.Warn("Unexpected response from agent")
			return &actorproto.LameDuckResponse{Id: ctx.Self().ID(), Success: false}
		}
		if !resp.Success {
			s.logger.Error("Child workload failed to shutdown", slog.String("child", pid.ID()), slog.Any("err", err))
			return &actorproto.LameDuckResponse{Id: ctx.Self().ID(), Success: false}
		}
	}
	return &actorproto.LameDuckResponse{Id: ctx.Self().ID(), Success: true}
}
