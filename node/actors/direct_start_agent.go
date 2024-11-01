package actors

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/nats-io/nuid"
	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"

	actorproto "github.com/synadia-io/nex/node/actors/pb"
)

const (
	DirectStartActorName = "direct_start"
	DirectStartActorDesc = "Direct start agent"

	VERSION = "0.0.0"
)

func CreateDirectStartAgent(options models.NodeOptions, logger *slog.Logger) *DirectStartAgent {
	return &DirectStartAgent{options: options, logger: logger}
}

type DirectStartAgent struct {
	self *goakt.PID

	options models.NodeOptions
	logger  *slog.Logger
}

func (a *DirectStartAgent) PreStart(ctx context.Context) error {

	return nil
}

func (a *DirectStartAgent) PostStop(ctx context.Context) error {
	return nil
}

func (a *DirectStartAgent) Receive(ctx *goakt.ReceiveContext) {
	switch m := ctx.Message().(type) {
	case *goaktpb.PostStart:
		a.self = ctx.Self()
		a.logger.Info("Direct start agent is running", slog.String("name", ctx.Self().Name()))
	case *actorproto.StartWorkload:

		env := make(map[string]string)
		if m.Environment != "" {
			env_b, err := base64.StdEncoding.DecodeString(m.Environment)
			if err != nil {
				a.logger.Error("Failed to decode environment", slog.String("name", ctx.Self().Name()), slog.Any("err", err))
				ctx.Err(err)
				return
			}

			err = json.Unmarshal(env_b, &env)
			if err != nil {
				a.logger.Error("Failed to unmarshal environment", slog.String("name", ctx.Self().Name()), slog.Any("err", err))
				ctx.Err(err)
				return
			}
		}

		ref, err := getArtifact(m.WorkloadName, m.Uri)
		if err != nil {
			a.logger.Error("Failed to get artifact", slog.String("name", ctx.Self().Name()), slog.Any("err", err))
			ctx.Err(err)
			return
		}

		workloadId := nuid.New().Next()
		pa, err := createNewProcessActor(a.logger.WithGroup(workloadId), workloadId, m, ref, env)
		if err != nil {
			a.logger.Error("Failed to create process actor", slog.String("name", ctx.Self().Name()), slog.Any("err", err))
			ctx.Err(err)
			return
		}

		c, err := a.self.SpawnChild(context.Background(), workloadId, pa)
		if err != nil {
			a.logger.Error("Failed to spawn child", slog.String("name", ctx.Self().Name()), slog.String("workload", m.WorkloadName), slog.Any("err", err))
			ctx.Err(err)
			return
		}
		a.logger.Debug("Spawned direct start process", slog.String("name", c.Name()))

		ctx.Response(&actorproto.WorkloadStarted{Id: workloadId, Name: m.WorkloadName, Started: true})
	case *actorproto.StopWorkload:
		a.logger.Debug("StopWorkload received", slog.String("name", ctx.Self().Name()), slog.String("workload", m.WorkloadId))
		for _, c := range a.self.Children() {
			if c.Name() == m.WorkloadId {
				a.logger.Debug("workload found; shutting down", slog.String("name", ctx.Self().Name()), slog.String("workload", c.Name()))

				err := c.Tell(context.Background(), c, &actorproto.KillDirectStartProcess{})
				if err != nil {
					a.logger.Error("failed to query workload", slog.String("name", ctx.Self().Name()), slog.Any("err", err))
					ctx.Err(err)
					return
				}

				err = c.Shutdown(context.Background())
				if err != nil {
					a.logger.Error("failed to stop workload", slog.String("name", ctx.Self().Name()), slog.String("workload", c.Name()), slog.Any("err", err))
					ctx.Err(err)
					return
				}
				ctx.Response(&actorproto.WorkloadStopped{Id: m.WorkloadId, Stopped: true})
				return
			}
		}
		a.logger.Error("workload not found", slog.String("name", ctx.Self().Name()), slog.String("workload", m.WorkloadId))
		ctx.Err(errors.New("workload not found"))
	case *actorproto.QueryWorkloads:
		ret := new(actorproto.WorkloadList)
		for _, c := range a.self.Children() {
			askRet, err := c.Ask(context.Background(), c, &actorproto.QueryWorkload{})
			if err != nil {
				a.logger.Error("failed to query workload", slog.String("name", ctx.Self().Name()), slog.Any("err", err))
				ctx.Err(err)
				return
			}
			workloadSummary, ok := askRet.(*actorproto.WorkloadSummary)
			if !ok {
				a.logger.Error("query workload unexpected response type", slog.String("name", ctx.Self().Name()), slog.String("workload", c.Name()))
				ctx.Err(errors.New("unexpected response type"))
				return
			}
			ret.Workloads = append(ret.Workloads, workloadSummary)
		}
		ctx.Response(ret)
	case *goaktpb.Terminated:
	default:
		a.logger.Warn("Direct start agent received unhandled message", slog.String("name", ctx.Self().Name()), slog.Any("message_type", fmt.Sprintf("%T", m)))
		ctx.Unhandled()
	}
}
