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

	workloadType string
	namespace    string
	processName  string

	logger  *slog.Logger
	self    *goakt.PID
	process *OsProcess
}

func createNewProcessActor(
	logger *slog.Logger,
	ncLog *nats.Conn,
	processId string,
	argv []string,
	namespace string,
	workloadType string,
	processName string,
	ref *ArtifactReference,
	env map[string]string) (*processActor, error) {

	ret := new(processActor)
	ret.processName = processName
	ret.workloadType = workloadType
	ret.namespace = namespace
	var err error

	stdout := logCapture{logger: logger, nc: ncLog, namespace: namespace, name: processId, stderr: false}
	stderr := logCapture{logger: logger, nc: ncLog, namespace: namespace, name: processId, stderr: true}

	ret.process, err = NewOsProcess(processId, ref.LocalCachePath, env, argv, logger, stdout, stderr)
	if err != nil {
		return nil, err
	}

	ret.id = processId
	ret.logger = logger
	ret.startedAt = time.Now()
	return ret, nil
}

func (a *processActor) PreStart(ctx context.Context) error {
	return nil
}

func (a *processActor) PostStop(ctx context.Context) error {
	a.logger.Debug("OS Process Actor stopped", slog.String("id", a.id))
	return nil
}

func (a *processActor) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		a.logger.Info("OS Process actor started", slog.String("name", a.processName))
		a.self = ctx.Self()
		ctx.Tell(ctx.Self(), &actorproto.SpawnDirectStartProcess{})
	case *actorproto.SpawnDirectStartProcess:
		a.logger.Debug("Spawning child proicess", slog.String("name", a.processName))
		go a.SpawnOsProcess(ctx)
	case *actorproto.KillDirectStartProcess:
		err := a.KillOsProcess()
		if err != nil {
			ctx.Err(err)
			return
		}

		ctx.Shutdown()
	case *actorproto.QueryWorkload:
		// NOTE: not all processes are workloads. One might be an agent
		ctx.Response(&actorproto.WorkloadSummary{
			Id:           a.id,
			Name:         a.processName,
			StartedAt:    timestamppb.New(a.startedAt),
			WorkloadType: a.workloadType,
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

	// waits 5 seconds for process to shutdown gracefully
	shutdownStart := time.Now()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for _ = range ticker.C {
		if !a.process.IsRunning() {
			ticker.Stop()
			a.logger.Debug("process stopped gracefully", slog.String("id", a.id))
			return nil
		}
		if time.Since(shutdownStart) > 5*time.Second {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	err = a.process.Kill()
	if err != nil {
		a.logger.Error("failed to kill process", slog.Any("err", err))
		return err
	}
	a.logger.Debug("process hard killed", slog.String("id", a.id))
	return nil
}
