package actors

import (
	"context"

	"github.com/tochemey/goakt/v2/goaktpb"
	"github.com/tochemey/goakt/v2/log"

	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v2/actors"
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
	// TODO: figure out what to do about logging
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
		s.logger.Infof("Agent supervisor '%v' is running", ctx.Self().Name())
	default:
		ctx.Unhandled()
	}

}
