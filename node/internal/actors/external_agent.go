package actors

import (
	"context"
	"log/slog"

	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"

	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
)

type ExternalAgent struct {
	agentOptions      models.AgentOptions
	internalNatsCreds AgentCredential
	logger            *slog.Logger
}

func CreateExternalAgent(logger *slog.Logger, creds AgentCredential, agentOptions models.AgentOptions) *ExternalAgent {
	return &ExternalAgent{
		agentOptions:      agentOptions,
		internalNatsCreds: creds,
		logger:            logger,
	}
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
		a.logger.Info("External agent for workload type is running", slog.String("name", ctx.Self().Name()))
	case *actorproto.QueryWorkloads:
		a.queryWorkloads(ctx)
	default:
		ctx.Unhandled()
	}
}

func (a *ExternalAgent) queryWorkloads(ctx *goakt.ReceiveContext) {
	// TODO: make this real
}
