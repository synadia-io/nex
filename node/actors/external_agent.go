package actors

import (
	"context"

	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
	"github.com/tochemey/goakt/v2/log"

	actorproto "github.com/synadia-io/nex/node/actors/pb"
)

type ExternalAgent struct {
	agentOptions      models.AgentOptions
	internalNatsCreds AgentCredential
	logger            log.Logger
}

func CreateExternalAgent(creds AgentCredential, agentOptions models.AgentOptions) *ExternalAgent {
	return &ExternalAgent{agentOptions: agentOptions, internalNatsCreds: creds}
}

func (a *ExternalAgent) PreStart(ctx context.Context) error {
	return nil
}

func (a *ExternalAgent) PostStop(ctx context.Context) error {
	return nil
}

func (a *ExternalAgent) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		a.logger = ctx.Self().Logger()
		a.logger.Infof("External agent for workload type '%s' is running", ctx.Self().Name())
	case *actorproto.QueryWorkloads:
		a.queryWorkloads(ctx)
	default:
		ctx.Unhandled()
	}
}

func (a *ExternalAgent) queryWorkloads(ctx *goakt.ReceiveContext) {
	// TODO: make this real
}
