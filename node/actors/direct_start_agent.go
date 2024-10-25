package actors

import (
	"context"

	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
	"github.com/tochemey/goakt/v2/log"
)

const DirectStartActorName = "direct_start"

func CreateDirectStartAgent(options models.NodeOptions) *DirectStartAgent {
	return &DirectStartAgent{options: options}
}

type DirectStartAgent struct {
	options models.NodeOptions
	logger  log.Logger
}

func (a *DirectStartAgent) PreStart(ctx context.Context) error {

	return nil
}

func (a *DirectStartAgent) PostStop(ctx context.Context) error {
	return nil
}

func (a *DirectStartAgent) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		a.logger = ctx.Self().Logger()
		a.logger.Infof("Direct start agent '%v' is running", ctx.Self().Name())
	default:
		ctx.Unhandled()
	}
}
