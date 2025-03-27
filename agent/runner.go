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
	ctx          context.Context
	name         string
	registerType string // this is used in the --type field for workloads
	version      string
	logger       *slog.Logger

	nodeId   string
	agentId  string
	triggers map[string]*nats.Subscription

	agent Agent
	nc    *nats.Conn
	micro micro.Service

	// Ingress Settings
	ingressHostMachineIpAddr string
	ingressReportingSubject  string
}

type RunnerOpt func(*Runner) error

func WithLogger(logger *slog.Logger) RunnerOpt {
	return func(a *Runner) error {
		a.logger = logger
		return nil
	}
}

func WithIngressSettings(hostMachineIpAddr, ingressReportingSubject string) RunnerOpt {
	return func(a *Runner) error {
		a.ingressHostMachineIpAddr = hostMachineIpAddr
		a.ingressReportingSubject = ingressReportingSubject
		return nil
	}
}

func RemoteAgentInit(nc *nats.Conn, pubKey string) (*models.RegisterRemoteAgentResponse, error) {
	regResp, err := nc.Request(models.RegisterRemoteAgentSubject(), fmt.Appendf([]byte{}, `{"public_signing_key":"%s"}`, pubKey), time.Second*3)
	if err != nil {
		return nil, err
	}

	var resp models.RegisterRemoteAgentResponse
	err = json.Unmarshal(regResp.Data, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func NewRunner(ctx context.Context, nodeId string, na Agent, opts ...RunnerOpt) (*Runner, error) {
	a := &Runner{
		ctx:          ctx,
		name:         "default",
		registerType: "default",
		version:      "0.0.0",
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),

		nodeId:   nodeId,
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
	return fmt.Sprintf("%s-%s", a.registerType, a.name)
}

func (a *Runner) GetLogger(workloadId, namespace string, lType LogType) io.Writer {
	return NewAgentLogCapture(a.nc, slog.Default(), lType, a.agentId, workloadId, namespace)
}

func (a *Runner) Run(agentId string, connData models.NatsConnectionData) error {
	var err error
	a.agentId = agentId

	a.nc, err = configureNatsConnection(connData)
	if err != nil {
		return err
	}

	// Register the agent
	register, err := a.agent.Register()
	if err != nil {
		return err
	}

	a.name = register.Name
	a.registerType = register.RegisterType
	a.version = register.Version

	registerB, err := json.Marshal(register)
	if err != nil {
		return err
	}

	var regRet *nats.Msg

	regRet, err = a.nc.Request(models.AgentAPILocalRegisterRequestSubject(agentId, a.nodeId), registerB, time.Second*3)
	if err != nil {
		return err
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

	a.nc, err = configureNatsConnection(regRetJson.ConnectionData)
	if err != nil {
		return err
	}

	// Start agent heartbeat
	go func() {
		for range time.Tick(time.Second * 10) {
			hb, err := a.agent.Heartbeat()
			if err != nil {
				a.logger.Warn("error generating heartbeat", slog.Any("err", err))
				continue
			}
			hbB, err := json.Marshal(hb)
			if err != nil {
				a.logger.Warn("error marshalling heartbeat", slog.Any("err", err))
				continue
			}
			err = a.nc.Publish(models.AgentAPIHeartbeatSubject(a.agentId), hbB)
			if err != nil {
				a.logger.Warn("error publishing heartbeat", slog.Any("err", err))
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

	type endpoint struct {
		Name    string
		Subject string
		Handler micro.HandlerFunc
	}

	// Start subscriptions to the host
	endpoints := []endpoint{
		{Name: "StartWorkload", Subject: models.AgentAPIStartWorkloadSubscribeSubject(a.nodeId, a.agentId), Handler: a.handleStartWorkload()},
		{Name: "StopWorkload", Subject: models.AgentAPIStopWorkloadSubscribeSubject(a.nodeId), Handler: a.handleStopWorkload()},
		{Name: "GetWorkload", Subject: models.AgentAPIGetWorkloadSubscribeSubject(a.nodeId), Handler: a.handleGetWorkload()},
		{Name: "QueryWorkloads", Subject: models.AgentAPIQueryWorkloadsSubject(a.nodeId), Handler: a.handleQueryWorkloads()},
		// System only endpoints
		{Name: "PingAgent", Subject: models.AgentAPIPingSubject(a.nodeId, a.agentId), Handler: a.handlePing()},
		{Name: "PingAllAgents", Subject: models.AgentAPIPingAllSubject(a.nodeId), Handler: a.handlePing()},
		{Name: "SetLameduck", Subject: models.AgentAPISetLameduckSubject(a.nodeId), Handler: a.handleSetLameduck()},
	}

	if _, ok := a.agent.(AgentIngessWorkloads); ok {
		endpoints = append(endpoints, endpoint{Name: "WorkloadDiscovery", Subject: models.AgentAPIPingWorkloadSubscribeSubject(), Handler: a.handleDiscoverWorkload()})
	}

	if _, ok := a.agent.(AgentEventListener); ok {
		endpoints = append(endpoints, endpoint{Name: "EventListener", Subject: fmt.Sprintf("$NEX.agent.%s.EVENT", a.agentId), Handler: a.handleReceivedEvent()})
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

	if regRetJson.ExistingState != nil {
		a.logger.Info("restoring existing state", slog.Int("num_workloads", len(regRetJson.ExistingState)))
		for workloadId, startRequest := range regRetJson.ExistingState {
			_, err = a.agent.StartWorkload(workloadId, &startRequest, true)
			if err != nil {
				a.logger.Error("error restoring existing state", slog.String("workload_id", workloadId), slog.Any("err", err))
			}
		}
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

func (a *Runner) RegisterTrigger(workloadId, triggerSubject string, workloadConnData *models.NatsConnectionData, tFunc func([]byte) ([]byte, error)) error {
	nc, err := configureNatsConnection(*workloadConnData)
	if err != nil {
		return err
	}

	sub, err := nc.Subscribe(triggerSubject, func(m *nats.Msg) {
		go func() {
			ret, err := tFunc(m.Data)
			if err != nil {
				a.logger.Error("error running trigger function", slog.Any("err", err))
			}
			if m.Reply != "" { // empty if orginal trigger was a publish and not request
				msg := &nats.Msg{
					Subject: m.Reply,
					Header:  nats.Header{"workload_id": []string{workloadId}},
					Data:    ret,
				}
				err = a.nc.PublishMsg(msg)
				if err != nil {
					a.logger.Error("error responding to trigger", slog.Any("err", err))
				}
			}
		}()
	})
	if err != nil {
		return err
	}

	a.triggers[workloadId] = sub
	return nil
}

func (a *Runner) RegisterTriggerWithAgent(namespace, workloadId string, tFunc func([]byte) ([]byte, error)) error {
	sub, err := a.nc.Subscribe(fmt.Sprintf("%s.%s.%s.TRIGGER", models.WorkloadAPIPrefix, namespace, workloadId), func(m *nats.Msg) {
		go func() {
			ret, err := tFunc(m.Data)
			if err != nil {
				a.logger.Error("error running trigger function", slog.Any("err", err))
			}
			if m.Reply != "" { // empty if orginal trigger was a publish and not request
				err = a.nc.Publish(m.Reply, ret)
				if err != nil {
					a.logger.Error("error responding to trigger", slog.Any("err", err))
				}
			}
		}()
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
			a.logger.Error("error unmarshalling start workload request", slog.Any("err", err), slog.String("data", string(r.Data())))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
			return
		}

		startResp, err := a.agent.StartWorkload(workloadId, req, false)
		if err != nil {
			a.logger.Error("failed to start workload", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
			return
		}

		err = r.RespondJSON(startResp)
		if err != nil {
			a.logger.Error("error responding to start workload request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
			return
		}

		if aiw, ok := a.agent.(AgentIngessWorkloads); ok {
			if a.ingressHostMachineIpAddr == "" || a.ingressReportingSubject == "" {
				a.logger.Warn("ingress data requested but not provided; settings missing", slog.String("host_machine_ip_addr", a.ingressHostMachineIpAddr), slog.String("reporting_subject", a.ingressReportingSubject))
				return
			}

			ports, err := aiw.GetWorkloadExposedPorts(workloadId)
			if err != nil {
				a.logger.Error("error getting exposed ports", slog.Any("err", err))
			}

			for i, port := range ports {
				ingressMsg := models.AgentIngressMsg{
					WorkloadId: workloadId,
					Upstream:   fmt.Sprintf("%s:%d", a.ingressHostMachineIpAddr, port),
					Command:    "add",
				}
				ingressMsgB, err := json.Marshal(ingressMsg)
				if err != nil {
					a.logger.Error("error marshalling ingress message", slog.Any("err", err))
					return
				}
				err = a.nc.Publish(a.ingressReportingSubject, ingressMsgB)
				if err != nil {
					a.logger.Error("error publishing to update_caddy", slog.Any("err", err))
				}

				if i == 0 {
					break // only first port for now
				}
			}
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
			a.logger.Error("error unmarshalling stop workload request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
		}

		err = a.agent.StopWorkload(workloadId, req)
		if err != nil && err.Error() == "workload not found" { // TODO: this is a hack for native
			return
		}
		if err != nil {
			a.logger.Error("error stopping workload request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
		}

		ret := models.StopWorkloadResponse{
			Id:           workloadId,
			Message:      "Success",
			Stopped:      true,
			WorkloadType: a.registerType,
		}

		err = r.RespondJSON(ret)
		if err != nil {
			a.logger.Error("error responding to stop workload request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
		}

		if sub, ok := a.triggers[workloadId]; ok {
			err := sub.Unsubscribe()
			if err != nil {
				a.logger.Error("error unsubscribing trigger", slog.Any("err", err))
			}
		}

		if _, ok := a.agent.(AgentIngessWorkloads); ok {
			if a.ingressHostMachineIpAddr == "" || a.ingressReportingSubject == "" {
				a.logger.Warn("ingress data requested but not provided; settings missing", slog.String("host_machine_ip_addr", a.ingressHostMachineIpAddr), slog.String("reporting_subject", a.ingressReportingSubject))
				return
			}

			ingressMsg := models.AgentIngressMsg{
				WorkloadId: workloadId,
				Command:    "remove",
			}
			ingressMsgB, err := json.Marshal(ingressMsg)
			if err != nil {
				a.logger.Error("error marshalling ingress message", slog.Any("err", err))
				return
			}
			err = a.nc.Publish(a.ingressReportingSubject, ingressMsgB)
			if err != nil {
				a.logger.Error("error publishing to update_caddy", slog.Any("err", err))
			}

		}
	}
}

func (a *Runner) handleGetWorkload() func(r micro.Request) {
	return func(r micro.Request) {
		// $NEX.agent.<nodeid>.GETWORKLOAD.workloadId
		splitSub := strings.SplitN(r.Subject(), ".", 5)
		workloadId := splitSub[4]

		// TODO: implement message with xkey
		startRequest, err := a.agent.GetWorkload(workloadId, "")
		if err != nil && err.Error() == "workload not found" {
			return
		}
		if err != nil {
			a.logger.Error("error getting workload", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to get workload request", slog.Any("err", err))
			}
			return
		}

		err = r.RespondJSON(startRequest)
		if err != nil {
			a.logger.Error("error responding to get workload request", slog.Any("err", err))
		}
	}
}

func (a *Runner) handleQueryWorkloads() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.agent.<nodeid>.QUERYWORKLOADS
		req := new(models.AgentListWorkloadsRequest)

		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			a.logger.Error("error unmarshalling query workloads request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
		}

		wl, err := a.agent.QueryWorkloads(req.Namespace, req.Filter)
		if err != nil {
			a.logger.Error("error querying workloads", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
		}

		err = r.RespondJSON(wl)
		if err != nil {
			a.logger.Error("error responding to query workloads request", slog.Any("err", err))
		}
	}
}

func (a *Runner) handleSetLameduck() func(micro.Request) {
	return func(r micro.Request) {
		req := new(models.LameduckRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			a.logger.Error("error unmarshalling set lameduck request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
			err = r.RespondJSON(models.LameduckResponse{Success: false})
			if err != nil {
				a.logger.Error("error responding to set lameduck request", slog.Any("err", err))
				err = r.Error("100", err.Error(), nil)
				if err != nil {
					a.logger.Error("error responding to start workload request", slog.Any("err", err))
				}
			}
			return
		}

		delay, err := time.ParseDuration(req.Delay)
		if err != nil {
			a.logger.Error("error parsing delay", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
			err = r.RespondJSON(models.LameduckResponse{Success: false})
			if err != nil {
				a.logger.Error("error responding to set lameduck request", slog.Any("err", err))
				err = r.Error("100", err.Error(), nil)
				if err != nil {
					a.logger.Error("error responding to start workload request", slog.Any("err", err))
				}
			}
			return
		}

		err = a.agent.SetLameduck(delay)
		if err != nil {
			a.logger.Error("error setting lameduck", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
			err = r.RespondJSON(models.LameduckResponse{Success: false})
			if err != nil {
				a.logger.Error("error responding to set lameduck request", slog.Any("err", err))
				err = r.Error("100", err.Error(), nil)
				if err != nil {
					a.logger.Error("error responding to start workload request", slog.Any("err", err))
				}
			}
			return
		}

		err = r.RespondJSON(models.LameduckResponse{Success: true})
		if err != nil {
			a.logger.Error("error responding to set lameduck request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
		}
	}
}

func (a *Runner) handlePing() func(micro.Request) {
	return func(r micro.Request) {
		resp, err := a.agent.Ping()
		if err != nil {
			a.logger.Error("error pinging agent", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
		}

		h := make(map[string][]string)
		h["agentId"] = []string{a.agentId}

		err = r.RespondJSON(resp, micro.WithHeaders(micro.Headers(h)))
		if err != nil {
			a.logger.Error("error responding to ping agent request", slog.Any("err", err))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.Any("err", err))
			}
		}
	}
}

// This response needs to be as quick as possible
func (a *Runner) handleDiscoverWorkload() func(micro.Request) {
	return func(r micro.Request) {
		ingressAgent, ok := a.agent.(AgentIngessWorkloads)
		if !ok {
			return
		}

		//$NEX.agent.PINGWORKLOAD.<workloadid>
		subSplit := strings.SplitN(r.Subject(), ".", 4)
		workloadId := subSplit[3]

		found := ingressAgent.PingWorkload(workloadId)
		if !found {
			return
		}

		err := r.Respond(fmt.Appendf([]byte{}, `{"node_id":"%s"}`, a.nodeId))
		if err != nil {
			a.logger.Error("error responding to workload ping", slog.Any("err", err))
		}
	}
}

func (a *Runner) handleReceivedEvent() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.agent.<agentid>.EVENT
		aE, ok := a.agent.(AgentEventListener)
		if !ok {
			return
		}
		aE.EventListener(r.Data())
	}
}
