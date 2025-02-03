package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/synadia-labs/nex/models"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
)

const (
	EnvVarPrefix = "NEX_AGENT"
)

type Runner struct {
	name    string
	version string

	remote   bool
	nodeId   string
	agentId  string
	triggers map[string]*nats.Subscription

	agent Agent
	nc    *nats.Conn
	micro micro.Service
}

type RunnerOpt func(*Runner) error

func AsLocalAgent(inNodeId string) RunnerOpt {
	return func(a *Runner) error {
		a.remote = false
		a.nodeId = inNodeId
		return nil
	}
}

func WithLogger(logger *slog.Logger) RunnerOpt {
	return func(a *Runner) error {
		slog.SetDefault(logger)
		return nil
	}
}

func NewRunner(name, version string, na Agent, opts ...RunnerOpt) (*Runner, error) {
	a := &Runner{
		name:     name,
		version:  version,
		agent:    na,
		triggers: make(map[string]*nats.Subscription),
	}

	for _, opt := range opts {
		if err := opt(a); err != nil {
			return nil, err
		}
	}

	return a, nil
}

func (a *Runner) String() string {
	return a.name
}

func (a *Runner) GetLogger(workloadId, namespace string, stderr bool) io.Writer {
	return NewAgentLogCapture(a.nc, slog.Default(), stderr, a.agentId, workloadId, namespace)
}

func (a *Runner) Run(ctx context.Context, agentId string, connData models.NatsConnectionData) error {
	var err error
	a.agentId = agentId

	a.nc, err = nats.Connect(connData.NatsUrl,
		nats.UserJWTAndSeed(connData.NatsUserJwt, connData.NatsUserSeed),
		nats.Name(a.agentId),
	)
	if err != nil {
		return err
	}

	// Register the agent
	register, err := a.agent.Register(agentId)
	if err != nil {
		return err
	}

	registerB, err := json.Marshal(register)
	if err != nil {
		return err
	}

	var regRet *nats.Msg
	if a.remote {
		regRet, err = a.nc.Request(models.AgentAPIRemoteRegisterSubject(), registerB, time.Second*3)
		if err != nil {
			return err
		}
	} else {
		regRet, err = a.nc.Request(models.AgentAPILocalRegisterRequestSubject(agentId, a.nodeId), registerB, time.Second*3)
		if err != nil {
			return err
		}
	}

	var regRetJson models.RegisterAgentResponse
	err = json.Unmarshal(regRet.Data, &regRetJson)
	if err != nil {
		return err
	}

	a.nc.Close()

	if !regRetJson.Success {
		return errors.New("agent registration failed: " + regRetJson.Message)
	}

	a.nc, err = nats.Connect(
		regRetJson.ConnectionData.NatsUrl,
		nats.UserJWTAndSeed(regRetJson.ConnectionData.NatsUserJwt, regRetJson.ConnectionData.NatsUserSeed),
		nats.Name(a.agentId),
	)
	if err != nil {
		return err
	}

	// Start agent heartbeat
	go func() {
		ticker := time.NewTicker(time.Second * 10)
		for range ticker.C {
			hb, err := a.agent.Heartbeat()
			if err != nil {
				slog.Warn("error generating heartbeat", slog.Any("err", err))
				continue
			}
			hbB, err := json.Marshal(hb)
			if err != nil {
				slog.Warn("error marshalling heartbeat", slog.Any("err", err))
				continue
			}
			err = a.nc.Publish(models.AgentAPIHeartbeatSubject(a.agentId), hbB)
			if err != nil {
				slog.Warn("error publishing heartbeat", slog.Any("err", err))
				continue
			}
		}
	}()

	a.micro, err = micro.AddService(a.nc, micro.Config{
		Name:    a.name,
		Version: a.version,
	})
	if err != nil {
		return err
	}

	// Start subscriptions to the host
	endpoints := []struct {
		Name    string
		Subject string
		Handler micro.HandlerFunc
	}{
		{Name: "StartWorkload", Subject: models.AgentAPIStartWorkloadSubscribeSubject(a.nodeId, a.agentId), Handler: a.handleStartWorkload()},
		{Name: "StopWorkload", Subject: models.AgentAPIStopWorkloadSubscribeSubject(a.nodeId), Handler: a.handleStopWorkload()},
		{Name: "QueryWorkloads", Subject: models.AgentAPIQueryWorkloadsSubject(a.nodeId), Handler: a.handleQueryWorkloads()},
		// System only endpoints
		{Name: "PingAgent", Subject: models.AgentAPIPingSubject(a.nodeId, a.agentId), Handler: a.handlePing()},
		{Name: "PingAllAgents", Subject: models.AgentAPIPingAllSubject(a.nodeId), Handler: a.handlePing()},
		{Name: "SetLameduck", Subject: models.AgentAPISetLameduckSubject(a.nodeId), Handler: a.handleSetLameduck()},
	}

	var errs error
	for _, ep := range endpoints {
		err := a.micro.AddEndpoint(ep.Name, ep.Handler, micro.WithEndpointSubject(ep.Subject), micro.WithEndpointQueueGroup(agentId))
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if errs != nil {
		return errs
	}

	return nil
}

func (a *Runner) Shutdown() error {
	return a.micro.Stop()
}

func (a *Runner) EmitEvent(event any) error {
	var eventB []byte
	var err error
	var eventType string

	switch event.(type) {
	case models.WorkloadStartedEvent:
		eventType = "WorkloadStarted"
	case models.WorkloadStoppedEvent:
		eventType = "WorkloadStopped"
	default:
		return errors.New("invalid event type")
	}

	eventB, err = json.Marshal(event)
	if err != nil {
		return err
	}
	return a.nc.Publish(models.AgentAPIEmitEventSubject(a.agentId, eventType), eventB)
}

func (a *Runner) RegisterTrigger(namespace, workloadId string, tFunc func([]byte) ([]byte, error)) error {
	sub, err := a.nc.Subscribe(fmt.Sprintf("%s.%s.%s.TRIGGER", models.WorkloadAPIPrefix, namespace, workloadId), func(m *nats.Msg) {
		ret, err := tFunc(m.Data)
		if err != nil {
			slog.Error("error running trigger function", slog.Any("err", err))
		}
		err = a.nc.Publish(m.Reply, ret)
		if err != nil {
			slog.Error("error responding to trigger", slog.Any("err", err))
		}
	})
	if err != nil {
		return err
	}

	a.triggers[workloadId] = sub
	return nil
}

func (a *Runner) handleStartWorkload() func(r micro.Request) {
	return func(r micro.Request) {
		// $NEX.agent.<namespace>.<agentid>.STARTWORKLOAD.<workloadid>
		splitSub := strings.SplitN(r.Subject(), ".", 6)
		workloadId := splitSub[5]

		req := new(models.AgentStartWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			slog.Error("error unmarshalling start workload request", slog.Any("err", err), slog.String("data", string(r.Data())))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
			return
		}

		startResp, err := a.agent.StartWorkload(workloadId, req)
		if err != nil {
			slog.Error("error unmarshalling start workload request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
			return
		}

		err = r.RespondJSON(startResp)
		if err != nil {
			slog.Error("error responding to start workload request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
			return
		}
	}
}

func (a *Runner) handleStopWorkload() func(r micro.Request) {
	return func(r micro.Request) {
		// return fmt.Sprintf("%s.%s.STOPWORKLOAD.*", inNodeId, AgentAPIPrefix)
		// $NEX.agent.<nodeid>.STOPWORKLOAD.workloadId
		splitSub := strings.SplitN(r.Subject(), ".", 5)
		workloadId := splitSub[4]

		req := new(models.StopWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			slog.Error("error unmarshalling stop workload request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
		}
		err = a.agent.StopWorkload(workloadId, req)
		if err != nil {
			slog.Error("error unmarshalling stop workload request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
		}

		ret := models.StopWorkloadResponse{
			Id:      workloadId,
			Message: "Success",
			Stopped: true,
		}

		err = r.RespondJSON(ret)
		if err != nil {
			slog.Error("error responding to stop workload request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
		}

		if sub, ok := a.triggers[workloadId]; ok {
			err := sub.Unsubscribe()
			if err != nil {
				slog.Error("error unsubscribing trigger", slog.Any("err", err))
			}
		}
	}
}

func (a *Runner) handleQueryWorkloads() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.agent.<nodeid>.QUERYWORKLOADS
		req := new(models.AgentListWorkloadsRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			slog.Error("error unmarshalling query workloads request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
		}

		wl, err := a.agent.QueryWorkloads(req.Namespace, req.Filter)
		if err != nil {
			slog.Error("error querying workloads", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
		}

		err = r.RespondJSON(wl)
		if err != nil {
			slog.Error("error responding to query workloads request", slog.Any("err", err))
		}
	}
}

func (a *Runner) handleSetLameduck() func(micro.Request) {
	return func(r micro.Request) {
		req := new(models.LameduckRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			slog.Error("error unmarshalling set lameduck request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
			err = r.RespondJSON(models.LameduckResponse{Success: false})
			if err != nil {
				slog.Error("error responding to set lameduck request", slog.Any("err", err))
				err = r.Error("100", err.Error(), nil)
				if err != nil {
					slog.Error("error responding to start workload request", slog.Any("err", err))
				}
			}
			return
		}

		delay, err := time.ParseDuration(req.Delay)
		if err != nil {
			slog.Error("error parsing delay", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
			err = r.RespondJSON(models.LameduckResponse{Success: false})
			if err != nil {
				slog.Error("error responding to set lameduck request", slog.Any("err", err))
				err = r.Error("100", err.Error(), nil)
				if err != nil {
					slog.Error("error responding to start workload request", slog.Any("err", err))
				}
			}
			return
		}

		err = a.agent.SetLameduck(delay)
		if err != nil {
			slog.Error("error setting lameduck", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
			err = r.RespondJSON(models.LameduckResponse{Success: false})
			if err != nil {
				slog.Error("error responding to set lameduck request", slog.Any("err", err))
				err = r.Error("100", err.Error(), nil)
				if err != nil {
					slog.Error("error responding to start workload request", slog.Any("err", err))
				}
			}
			return
		}

		err = r.RespondJSON(models.LameduckResponse{Success: true})
		if err != nil {
			slog.Error("error responding to set lameduck request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
		}
	}
}

func (a *Runner) handlePing() func(micro.Request) {
	return func(r micro.Request) {
		resp, err := a.agent.Ping()
		if err != nil {
			slog.Error("error pinging agent", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
		}

		h := make(map[string][]string)
		h["agentId"] = []string{a.agentId}

		err = r.RespondJSON(resp, micro.WithHeaders(micro.Headers(h)))
		if err != nil {
			slog.Error("error responding to ping agent request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				slog.Error("error responding to start workload request", slog.Any("err", err))
			}
		}
	}
}
