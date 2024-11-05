package actors

import (
	"context"

	"github.com/tochemey/goakt/v2/goaktpb"
	"github.com/tochemey/goakt/v2/log"

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
	logger log.Logger

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
		s.logger = ctx.Self().Logger()
	case *actorproto.QueryWorkloads:
		s.queryWorkloads(ctx)
	default:
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
