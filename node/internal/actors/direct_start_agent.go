package actors

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"disorder.dev/shandler"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nuid"
	"github.com/synadia-io/nex/models"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	actorproto "github.com/synadia-io/nex/node/internal/actors/pb"
)

const (
	DirectStartActorName = "direct_start"
	DirectStartActorDesc = "Direct start agent"

	VERSION = "0.0.0"
)

func CreateDirectStartAgent(nc *nats.Conn, nodeId string, options models.NodeOptions, logger *slog.Logger) *DirectStartAgent {
	return &DirectStartAgent{nc: nc, nodeId: nodeId, options: options, logger: logger}
}

type DirectStartAgent struct {
	self *goakt.PID

	nodeId    string
	startedAt time.Time
	nc        *nats.Conn
	options   models.NodeOptions
	logger    *slog.Logger
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
		a.startedAt = time.Now()
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
	case *actorproto.SetLameDuck:
		a.logger.Debug("SetLameDuck received", slog.String("name", ctx.Self().Name()))
		err := a.SetLameDuck()
		if err != nil {
			ctx.Response(&actorproto.LameDuckResponse{
				Success: false,
			})
			return
		}
		ctx.Response(&actorproto.LameDuckResponse{
			Success: true,
		})
	case *actorproto.PingWorkload:
		a.logger.Debug("PingWorkload received", slog.String("name", ctx.Self().Name()), slog.String("workload", m.WorkloadId))
		resp, err := a.pingWorkload(m.WorkloadId)
		if err != nil {
			// Pings dont respond negatively...they just dont respond
			return
		}
		ctx.Response(resp)
	case *actorproto.PingAgent:
		a.logger.Debug("PingAgent received", slog.String("name", ctx.Self().Name()))
		workloads, err := a.queryWorkloads(&actorproto.QueryWorkloads{})
		if err != nil {
			a.logger.Error("Failed to query workloads", slog.String("name", ctx.Self().Name()), slog.Any("err", err))
			ctx.Err(err)
			return
		}

		runningWorkloads := []*actorproto.RunningWorkload{}
		for _, w := range workloads.Workloads {
			runningWorkloads = append(runningWorkloads, &actorproto.RunningWorkload{
				Id:        w.Id,
				Namespace: "", // TODO: this is missing
				Name:      w.Name,
			})
		}
		ctx.Response(&actorproto.PingAgentResponse{
			NodeId:           a.nodeId,
			TargetXkey:       "",
			Version:          VERSION,
			Tags:             map[string]string{},
			StartedAt:        timestamppb.New(a.startedAt),
			RunningWorkloads: runningWorkloads,
		})
	case *goaktpb.Terminated:
		a.logger.Debug("Received terminated message", slog.String("actor", m.ActorId))
	default:
		a.logger.Warn("Direct start agent received unhandled message", slog.String("name", ctx.Self().Name()), slog.Any("message_type", fmt.Sprintf("%T", m)))
		ctx.Unhandled()
	}
}

func (a DirectStartAgent) pingWorkload(workloadId string) (*actorproto.PingWorkloadResponse, error) {
	wl, err := a.self.Child(workloadId)
	if err != nil {
		return nil, err
	}

	askResp, err := wl.Ask(context.Background(), wl, &actorproto.QueryWorkload{})
	if err != nil {
		a.logger.Log(context.Background(), shandler.LevelTrace, "Failed to query workload", slog.String("name", a.self.Name()), slog.String("workload", workloadId))
		return nil, err
	}
	workloadSummary, ok := askResp.(*actorproto.WorkloadSummary)
	if !ok {
		a.logger.Log(context.Background(), shandler.LevelTrace, "query workload unexpected response type", slog.String("name", a.self.Name()), slog.String("workload", wl.Name()))
		return nil, err
	}
	return &actorproto.PingWorkloadResponse{
		Workload: workloadSummary,
	}, nil
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
	ref, err := GetArtifact(m.WorkloadName, m.Uri, nil)
	if err != nil {
		a.logger.Error("Failed to get artifact", slog.String("name", a.self.Name()), slog.Any("err", err))
		return nil, err
	}

	workloadId := nuid.New().Next()
	pa, err := createNewProcessActor(
		a.logger.WithGroup("workload"),
		a.nc,
		workloadId,
		m.Argv,
		m.Namespace,
		m.WorkloadType,
		m.WorkloadName,
		ref,
		env)
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
		a.logger.Error("failed to stop workload", slog.String("name", a.self.Name()), slog.Any("err", err))
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

func (a *DirectStartAgent) SetLameDuck() error {
	for _, c := range a.self.Children() {
		err := c.Tell(context.Background(), c, &actorproto.KillDirectStartProcess{})
		if err != nil {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C:
			if len(a.self.Children()) == 0 {
				err := a.self.Shutdown(ctx)
				if err != nil {
					a.logger.Error("Failed to shutdown direct start agent", slog.String("name", a.self.Name()), slog.Any("err", err))
					return err
				}
				return nil
			}
		case <-ctx.Done():
			a.logger.Error("Failed to stop all workloads", slog.String("name", a.self.Name()))
			err := a.self.Shutdown(ctx)
			if err != nil {
				a.logger.Error("Failed to shutdown direct start agent", slog.String("name", a.self.Name()), slog.Any("err", err))
			}
			return errors.New("failed to stop all workloads in timelimit")
		}
	}
}
