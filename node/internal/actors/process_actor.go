package actors

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/models"
	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	DefaultJobRunTime = 5 * time.Minute
)

type processActor struct {
	startedAt time.Time
	runTime   time.Duration
	id        string
	nc        *nats.Conn

	argv         []string
	env          map[string]string
	workloadType string
	namespace    string
	processName  string
	runType      string
	state        string
	triggerSub   string
	retryCount   int
	ref          *ArtifactReference

	cancel  context.CancelFunc
	logger  *slog.Logger
	self    *goakt.PID
	process *OsProcess
}

func createNewProcessActor(
	logger *slog.Logger,
	nc *nats.Conn,
	processId string,
	argv []string,
	namespace string,
	workloadType string,
	processName string,
	ref *ArtifactReference,
	env map[string]string,
	runType string,
	triggerSub string,
	retryCount int,
) (*processActor, error) {

	ret := processActor{
		startedAt:    time.Now(),
		runTime:      0,
		id:           processId,
		nc:           nc,
		workloadType: workloadType,
		namespace:    namespace,
		processName:  processName,
		runType:      runType,
		state:        models.WorkloadStateInitializing,
		ref:          ref,
		logger:       logger,
		argv:         argv,
		env:          env,
		triggerSub:   triggerSub,
		retryCount:   retryCount,
	}

	return &ret, nil
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
		a.state = models.WorkloadStateStopped
		err := a.KillOsProcess()
		if err != nil {
			ctx.Err(err)
			return
		}

		if a.cancel != nil {
			a.cancel()
		}

		ctx.Shutdown()
	case *actorproto.QueryWorkload:
		ctx.Response(&actorproto.WorkloadSummary{
			Id:              a.id,
			Name:            a.processName,
			Namespace:       a.namespace,
			Runtime:         a.runTime.String(),
			StartedAt:       timestamppb.New(a.startedAt),
			WorkloadType:    a.workloadType,
			WorkloadRuntype: a.runType,
			State:           a.state,
		})
	default:
		a.logger.Warn("unknown message", slog.Any("msg", ctx.Message()))
		ctx.Unhandled()
	}
}

func (a *processActor) SpawnOsProcess(ctx *goakt.ReceiveContext) {
	var err error

	switch a.runType {
	case models.WorkloadRunTypeService:
		c := 0
		for a.state != models.WorkloadStateStopped && c < a.retryCount {
			a.state = models.WorkloadStateRunning

			stdout := logCapture{logger: a.logger, nc: a.nc, namespace: a.namespace, name: a.id, stderr: false}
			stderr := logCapture{logger: a.logger, nc: a.nc, namespace: a.namespace, name: a.id, stderr: true}

			a.process, err = NewOsProcess(a.id, a.ref.LocalCachePath, a.env, a.argv, a.logger, stdout, stderr)
			if err != nil {
				a.logger.Error("failed to create process", slog.Any("err", err))
				return
			}

			err = a.process.Run()
			if err != nil {
				a.logger.Error("failed to start process", slog.Any("err", err))
			}
			a.state = models.WorkloadStateError
			c++
		}

		if c == a.retryCount {
			a.logger.Error("failed to start process after retries", slog.Int("retryCount", a.retryCount))
		}
		ctx.Shutdown()
	case models.WorkloadRunTypeFunction:
		a.state = models.WorkloadStateWarm

		cctx, cancel := context.WithCancel(context.Background())
		a.cancel = cancel

		// TODO: subscribe to all triggers or change to only allow one
		// TODO: need to have a better quere group. possibly an ID at start time
		s, err := a.nc.QueueSubscribe(a.triggerSub, a.processName, func(msg *nats.Msg) {
			a.state = models.WorkloadStateRunning

			ticker := time.NewTicker(DefaultJobRunTime)
			go func() {
				<-ticker.C
				a.logger.Debug("Function run time exceeded", slog.String("id", a.id))
				err := a.KillOsProcess()
				if err != nil {
					a.logger.Error("failed to kill process", slog.Any("err", err))
				}
				a.state = models.WorkloadStateError
			}()

			stdout := logCapture{logger: a.logger, nc: a.nc, namespace: a.namespace, name: a.id, stderr: false}
			stderr := logCapture{logger: a.logger, nc: a.nc, namespace: a.namespace, name: a.id, stderr: true}

			if a.env == nil {
				a.env = make(map[string]string)
			}
			a.env["NEX_TRIGGER_DATA"] = string(msg.Data)
			a.process, err = NewOsProcess(a.id, a.ref.LocalCachePath, a.env, a.argv, a.logger, stdout, stderr)
			if err != nil {
				a.logger.Error("failed to create process", slog.Any("err", err))
				return
			}

			exeStart := time.Now()
			err = a.process.Run()
			if err != nil {
				a.logger.Error("failed to start process", slog.Any("err", err))
				ctx.Shutdown()
				return
			}
			exeEnd := time.Now()

			a.runTime = a.runTime + exeEnd.Sub(exeStart)
			a.state = models.WorkloadStateWarm
			ticker.Stop()
		})
		if err != nil {
			a.logger.Error("failed to subscribe to trigger", slog.Any("err", err))
			ctx.Shutdown()
			return
		}
		defer func() {
			_ = s.Unsubscribe()
		}()

		<-cctx.Done()
	case models.WorkloadRunTypeJob:
		a.state = models.WorkloadStateRunning

		stdout := logCapture{logger: a.logger, nc: a.nc, namespace: a.namespace, name: a.id, stderr: false}
		stderr := logCapture{logger: a.logger, nc: a.nc, namespace: a.namespace, name: a.id, stderr: true}

		a.process, err = NewOsProcess(a.id, a.ref.LocalCachePath, a.env, a.argv, a.logger, stdout, stderr)
		if err != nil {
			a.logger.Error("failed to create process", slog.Any("err", err))
			return
		}

		err = a.process.Run()
		if err != nil {
			a.logger.Error("failed to start process", slog.Any("err", err))
		}
		ctx.Shutdown()
	}
}

func (a *processActor) KillOsProcess() error {
	a.state = models.WorkloadStateStopped
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
