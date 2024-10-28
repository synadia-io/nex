package actors

import (
	"context"

	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
	"github.com/tochemey/goakt/v2/log"

	actorproto "github.com/synadia-io/nex/node/actors/pb"
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
	case *actorproto.QueryWorkloads:
		a.queryWorkloads(ctx)
	default:
		ctx.Unhandled()
	}
}

func (a *DirectStartAgent) queryWorkloads(ctx *goakt.ReceiveContext) {
	// TODO: make this real

	results := make([]*actorproto.WorkloadSummary, 0)
	listing := &actorproto.WorkloadListing{Workloads: results}
	ctx.Response(listing)
}
