package actors

import (
	"context"

	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
	"github.com/tochemey/goakt/v2/log"
)

const HostServicesActorName = "host_services"

func CreateHostServices(options models.HostServiceOptions) *HostServicesServer {
	return &HostServicesServer{options: options}
}

type HostServicesServer struct {
	options models.HostServiceOptions
	logger  log.Logger
}

func (s *HostServicesServer) PreStart(ctx context.Context) error {
	return nil
}

func (s *HostServicesServer) PostStop(ctx context.Context) error {
	return nil
}

func (s *HostServicesServer) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		s.logger = ctx.Self().Logger()
		s.logger.Infof("Host services server '%s' is running", ctx.Self().Name())
	default:
		ctx.Unhandled()
	}
}
