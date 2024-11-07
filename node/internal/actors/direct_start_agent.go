package actors

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nuid"
	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"

	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
)

const (
	DirectStartActorName = "direct_start"
	DirectStartActorDesc = "Direct start agent"

	VERSION = "0.0.0"
)

func CreateDirectStartAgent(nc *nats.Conn, options models.NodeOptions, logger *slog.Logger) *DirectStartAgent {
	return &DirectStartAgent{nc: nc, options: options, logger: logger}
}

type DirectStartAgent struct {
	self *goakt.PID

	nc      *nats.Conn
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
		a.logger.Debug("StartWorkload received", slog.String("name", ctx.Self().Name()), slog.String("workload", m.WorkloadName))
		resp, err := a.startWorkload(m)
		if err != nil {
			ctx.Err(err)
			return
		}
		ctx.Response(resp)
	case *actorproto.StopWorkload:
		a.logger.Debug("StopWorkload received", slog.String("name", ctx.Self().Name()), slog.String("workload", m.WorkloadId))
		resp, err := a.stopWorkload(m)
		if err != nil {
			ctx.Err(err)
			return
		}
		ctx.Response(resp)
	case *actorproto.QueryWorkloads:
		a.logger.Debug("QueryWorkloads received", slog.String("name", ctx.Self().Name()))
		resp, err := a.queryWorkloads(m)
		if err != nil {
			ctx.Err(err)
			return
		}
		ctx.Response(resp)
	case *goaktpb.Terminated:
	default:
		a.logger.Warn("Direct start agent received unhandled message", slog.String("name", ctx.Self().Name()), slog.Any("message_type", fmt.Sprintf("%T", m)))
		ctx.Unhandled()
	}
}

func (a *DirectStartAgent) startWorkload(m *actorproto.StartWorkload) (*actorproto.WorkloadStarted, error) {
	env := make(map[string]string)
	if m.Environment != "" {
		env_b, err := base64.StdEncoding.DecodeString(m.Environment)
		if err != nil {
			a.logger.Error("Failed to decode environment", slog.String("name", a.self.Name()), slog.Any("err", err))
			return nil, err
		}

		err = json.Unmarshal(env_b, &env)
		if err != nil {
			a.logger.Error("Failed to unmarshal environment", slog.String("name", a.self.Name()), slog.Any("err", err))
			return nil, err
		}
	}

	// TODO: figure out which nats connection to use here
	ref, err := getArtifact(m.WorkloadName, m.Uri, nil)
	if err != nil {
		a.logger.Error("Failed to get artifact", slog.String("name", a.self.Name()), slog.Any("err", err))
		return nil, err
	}

	workloadId := nuid.New().Next()
	pa, err := createNewProcessActor(a.logger.WithGroup("workload"), a.nc, workloadId, m, ref, env)
	if err != nil {
		a.logger.Error("Failed to create process actor", slog.String("name", a.self.Name()), slog.Any("err", err))
		return nil, err
	}

	c, err := a.self.SpawnChild(context.Background(), workloadId, pa)
	if err != nil {
		a.logger.Error("Failed to spawn child", slog.String("name", a.self.Name()), slog.String("workload", m.WorkloadName), slog.Any("err", err))
		return nil, err
	}

	a.logger.Info("Spawned direct start process", slog.String("name", c.Name()))
	return &actorproto.WorkloadStarted{Id: workloadId, Name: m.WorkloadName, Started: true}, nil
}

func (a *DirectStartAgent) stopWorkload(m *actorproto.StopWorkload) (*actorproto.WorkloadStopped, error) {
	c, err := a.self.Child(m.WorkloadId)
	if err != nil {
		a.logger.Error("workload not found", slog.String("name", a.self.Name()), slog.String("workload", m.WorkloadId))
		return nil, errors.New("workload not found")
	}

	err = c.Tell(context.Background(), c, &actorproto.KillDirectStartProcess{})
	if err != nil {
		a.logger.Error("failed to query workload", slog.String("name", a.self.Name()), slog.Any("err", err))
		return nil, err
	}

	a.logger.Info("Stopping direct start process", slog.String("name", c.Name()))
	return &actorproto.WorkloadStopped{Id: m.WorkloadId, Stopped: true}, nil
}

func (a *DirectStartAgent) queryWorkloads(m *actorproto.QueryWorkloads) (*actorproto.WorkloadList, error) {
	ret := new(actorproto.WorkloadList)
	for _, c := range a.self.Children() {
		askRet, err := c.Ask(context.Background(), c, &actorproto.QueryWorkload{})
		if err != nil {
			a.logger.Error("failed to query workload", slog.String("name", a.self.Name()), slog.Any("err", err))
			return nil, err
		}
		workloadSummary, ok := askRet.(*actorproto.WorkloadSummary)
		if !ok {
			a.logger.Error("query workload unexpected response type", slog.String("name", a.self.Name()), slog.String("workload", c.Name()))
			return nil, err
		}
		ret.Workloads = append(ret.Workloads, workloadSummary)
	}
	return ret, nil
}
