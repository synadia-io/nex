package actors

import (
	"context"
	"log/slog"
	"time"

	actorproto "github.com/synadia-io/nex/node/actors/pb"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type processActor struct {
	startedAt time.Time
	id        string

	startCommand *actorproto.StartWorkload

	logger  *slog.Logger
	self    *goakt.PID
	process *OsProcess
}

func createNewProcessActor(logger *slog.Logger, workloadId string, m *actorproto.StartWorkload, ref *ArtifactReference, env map[string]string) (*processActor, error) {
	ret := new(processActor)
	var err error
	ret.process, err = NewOsProcess(workloadId, ref.LocalCachePath, env, m.Argv, logger)
	if err != nil {
		return nil, err
	}

	ret.id = workloadId
	ret.logger = logger
	ret.startedAt = time.Now()
	ret.startCommand = m
	return ret, nil
}

func (a *processActor) PreStart(ctx context.Context) error {
	return nil
}

func (a *processActor) PostStop(ctx context.Context) error {
	return nil
}

func (a *processActor) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		a.self = ctx.Self()
		ctx.Tell(ctx.Self(), &actorproto.SpawnDirectStartProcess{})
	case *actorproto.SpawnDirectStartProcess:
		go func() {
			err := a.process.Run()
			if err != nil {
				a.logger.Error("failed to start process", slog.Any("err", err))
				ctx.Err(err)
				return
			}
		}()
	case *actorproto.KillDirectStartProcess:
		err := a.process.Stop("commanded")
		if err != nil {
			a.logger.Error("failed to stop process", slog.Any("err", err))
			ctx.Err(err)
			return
		}
	case *actorproto.QueryWorkload:
		ctx.Response(&actorproto.WorkloadSummary{
			Id:           a.id,
			Name:         a.startCommand.WorkloadName,
			StartedAt:    timestamppb.New(a.startedAt),
			WorkloadType: a.startCommand.WorkloadType,
		})
	default:
		a.logger.Warn("unknown message", slog.Any("msg", ctx.Message()))
		ctx.Unhandled()
	}
}
