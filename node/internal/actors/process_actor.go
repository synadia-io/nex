package actors

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type processActor struct {
	startedAt time.Time
	id        string
	namespace string

	startCommand *actorproto.StartWorkload

	logger  *slog.Logger
	self    *goakt.PID
	process *OsProcess
}

func createNewProcessActor(logger *slog.Logger, ncLog *nats.Conn, workloadId string, m *actorproto.StartWorkload, ref *ArtifactReference, env map[string]string) (*processActor, error) {
	ret := new(processActor)
	var err error

	stdout := logCapture{logger: logger, nc: ncLog, namespace: m.Namespace, name: workloadId, stderr: false}
	stderr := logCapture{logger: logger, nc: ncLog, namespace: m.Namespace, name: workloadId, stderr: true}

	ret.process, err = NewOsProcess(workloadId, ref.LocalCachePath, env, m.Argv, logger, stdout, stderr)
	if err != nil {
		return nil, err
	}

	ret.id = workloadId
	ret.logger = logger
	ret.startedAt = time.Now()
	ret.startCommand = m
	ret.namespace = m.Namespace
	return ret, nil
}

func (a *processActor) PreStart(ctx context.Context) error {
	return nil
}

func (a *processActor) PostStop(ctx context.Context) error {
	a.logger.Debug("Actor stopped", slog.String("id", a.id))
	return nil
}

func (a *processActor) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		a.self = ctx.Self()
		ctx.Tell(ctx.Self(), &actorproto.SpawnDirectStartProcess{})
	case *actorproto.SpawnDirectStartProcess:
		go a.SpawnOsProcess(ctx)
	case *actorproto.KillDirectStartProcess:
		err := a.KillOsProcess()
		if err != nil {
			ctx.Err(err)
			return
		}

		ctx.Shutdown()
	case *actorproto.QueryWorkload:
		ctx.Response(&actorproto.WorkloadSummary{
			Name:         a.startCommand.WorkloadName,
			StartedAt:    timestamppb.New(a.startedAt),
			WorkloadType: a.startCommand.WorkloadType,
			Namespace:    a.namespace,
		})
	default:
		a.logger.Warn("unknown message", slog.Any("msg", ctx.Message()))
		ctx.Unhandled()
	}
}

func (a *processActor) SpawnOsProcess(ctx *goakt.ReceiveContext) {
	err := a.process.Run()
	if err != nil {
		a.logger.Error("failed to start process", slog.Any("err", err))
		ctx.Shutdown()
		return
	}
}

func (a *processActor) KillOsProcess() error {
	err := a.process.Interrupt("commanded")
	if err != nil {
		a.logger.Error("failed to stop process", slog.Any("err", err))
		err := a.process.Kill()
		if err != nil {
			a.logger.Error("failed to kill process", slog.Any("err", err))
			return err
		}
	}

	// waits 5 seconds for workload to shutdown gracefully
	shutdownStart := time.Now()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for _ = range ticker.C {
		if !a.process.IsRunning() {
			ticker.Stop()
			a.logger.Debug("workload stopped gracefully", slog.String("id", a.id))
			return nil
		}
		if time.Since(shutdownStart) > 5*time.Second {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	err = a.process.Kill()
	if err != nil {
		a.logger.Error("failed to kill workload", slog.Any("err", err))
		return err
	}
	a.logger.Debug("workload hard killed", slog.String("id", a.id))
	return nil
}
