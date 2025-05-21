package actors

import (
	goakt "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/goaktpb"
	"github.com/tochemey/goakt/v3/log"

	"github.com/synadia-io/nex/models"
)

const HostServicesActorName = "host_services"

func CreateHostServices(options models.HostServiceOptions) *HostServicesServer {
	return &HostServicesServer{options: options}
}

type HostServicesServer struct {
	options models.HostServiceOptions
	logger  log.Logger
}

func (s *HostServicesServer) PreStart(*goakt.Context) error {
	return nil
}

func (s *HostServicesServer) PostStop(*goakt.Context) error {
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
