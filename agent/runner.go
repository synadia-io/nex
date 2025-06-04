package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	metrics      bool
	metricsPort  int

	nodeId   string
	nexus    string
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

// WithMetrics enables the prometheus metrics endpoint
// at http://localhost:<promPort>/metrics
// Default port: 9095
func WithPrometheusMetrics(promPort int) RunnerOpt {
	return func(a *Runner) error {
		if promPort > 0 && promPort <= 65535 {
			a.metricsPort = promPort
		}
		a.metrics = true
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

func RemoteAgentInit(nc *nats.Conn, nexus, pubKey string) (*models.RegisterRemoteAgentResponse, error) {
	req := models.RegisterRemoteAgentRequest{
		PublicSigningKey: pubKey,
	}

	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	regResp, err := nc.Request(models.AgentAPIInitRemoteRegisterRequestSubject(nexus, pubKey), req_b, time.Second*3)
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

func NewRunner(ctx context.Context, nexus, nodeId string, na Agent, opts ...RunnerOpt) (*Runner, error) {
	a := &Runner{
		ctx:          ctx,
		name:         "default",
		registerType: "default",
		version:      "0.0.0",
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		metrics:      false,
		metricsPort:  9095,

		nodeId:   nodeId,
		nexus:    nexus,
		agent:    na,
		agentId:  "default",
		triggers: make(map[string]*nats.Subscription),
	}

	for _, opt := range opts {
		if err := opt(a); err != nil {
			return nil, err
		}
	}

	return a, nil
}

func (a *Runner) ServiceIsRunning() bool {
	if a.micro == nil {
		return false
	}
	return !a.micro.Stopped()
}

func (a *Runner) String() string {
	return fmt.Sprintf("%s-%s", a.registerType, a.name)
}

func (a *Runner) GetLogger(workloadId, namespace string, lType models.LogOut) io.Writer {
	return NewAgentLogCapture(a.nc, slog.Default(), lType, a.agentId, workloadId, namespace)
}

func (a *Runner) Run(agentId string, connData models.NatsConnectionData) error {
	if a.metrics {
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			err := http.ListenAndServe(fmt.Sprintf(":%d", a.metricsPort), nil)
			if err != nil {
				a.logger.Error("failed to start metrics server", slog.String("err", err.Error()), slog.String("agent_id", agentId))
			}
		}()
	}

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

	regRet, err = a.nc.Request(models.AgentAPIRegisterRequestSubject(agentId, a.nodeId), registerB, time.Second*3)
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
				a.logger.Warn("error generating heartbeat", slog.String("err", err.Error()))
				continue
			}
			hbB, err := json.Marshal(hb)
			if err != nil {
				a.logger.Warn("error marshalling heartbeat", slog.String("err", err.Error()))
				continue
			}
			err = a.nc.Publish(models.AgentAPIHeartbeatSubject(a.nodeId, a.agentId), hbB)
			if err != nil {
				a.logger.Warn("error publishing heartbeat", slog.String("err", err.Error()))
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
		endpoints = append(endpoints, endpoint{Name: "WorkloadDiscovery", Subject: models.AgentAPIPingWorkloadSubscribeSubject(a.nexus), Handler: a.handleDiscoverWorkload()})
	}

	if _, ok := a.agent.(AgentEventListener); ok {
		endpoints = append(endpoints, endpoint{Name: "EventListener", Subject: models.EventAPIPrefix(a.agentId), Handler: a.handleReceivedEvent()})
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

	if len(regRetJson.ExistingState) > 0 {
		a.logger.Info("restoring existing state", slog.Int("num_workloads", len(regRetJson.ExistingState)))
		for workloadId, startRequest := range regRetJson.ExistingState {
			_, err = a.agent.StartWorkload(workloadId, &startRequest, true)
			if err != nil {
				a.logger.Error("error restoring existing state", slog.String("workload_id", workloadId), slog.String("err", err.Error()))
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
				a.logger.Error("error running trigger function", slog.String("err", err.Error()))
			}
			if m.Reply != "" { // empty if orginal trigger was a publish and not request
				msg := &nats.Msg{
					Subject: m.Reply,
					Header:  nats.Header{"workload_id": []string{workloadId}},
					Data:    ret,
				}
				err = a.nc.PublishMsg(msg)
				if err != nil {
					a.logger.Error("error responding to trigger", slog.String("err", err.Error()))
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

// This is used to unregister a trigger that was registered with the workload
// If a workload is stopped via successful StopWorkloadRequest, the trigger will be unregistered automatically
// If the workload fails to start inside a nexlet, use this function to clean up any unused triggers
func (a *Runner) UnregisterTrigger(workloadId string) error {
	sub, ok := a.triggers[workloadId]
	if !ok {
		a.logger.Debug("attempted to unregister a non-existent trigger", slog.String("workload_id", workloadId))
		return nil
	}

	err := sub.Unsubscribe()
	if err != nil {
		return fmt.Errorf("failed to unsubscribe trigger for workload %s: %w", workloadId, err)
	}

	delete(a.triggers, workloadId)
	return nil
}

func (a *Runner) handleStartWorkload() func(r micro.Request) {
	return func(r micro.Request) {
		splitSub := strings.SplitN(r.Subject(), ".", 7)
		workloadId := splitSub[6]

		req := new(models.AgentStartWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			handlerError(a.logger, r, err, "100", models.StartWorkloadResponse{
				Id:   workloadId,
				Name: req.Request.Name,
			})
			return
		}

		startResp, err := a.agent.StartWorkload(workloadId, req, false)
		if err != nil {
			handlerError(a.logger, r, err, "100", models.StartWorkloadResponse{
				Id:   workloadId,
				Name: req.Request.Name,
			})
			return
		}

		err = r.RespondJSON(startResp)
		if err != nil {
			handlerError(a.logger, r, err, "100", models.StartWorkloadResponse{
				Id:   workloadId,
				Name: req.Request.Name,
			})
			return
		}

		if aiw, ok := a.agent.(AgentIngessWorkloads); ok {
			if a.ingressHostMachineIpAddr == "" || a.ingressReportingSubject == "" {
				a.logger.Warn("ingress data requested but not provided; settings missing", slog.String("host_machine_ip_addr", a.ingressHostMachineIpAddr), slog.String("reporting_subject", a.ingressReportingSubject))
				return
			}

			ports, err := aiw.GetWorkloadExposedPorts(workloadId)
			if err != nil {
				a.logger.Error("error getting exposed ports", slog.String("err", err.Error()))
			}

			for i, port := range ports {
				ingressMsg := models.AgentIngressMsg{
					WorkloadId: workloadId,
					Upstream:   fmt.Sprintf("%s:%d", a.ingressHostMachineIpAddr, port),
					Command:    "add",
				}
				ingressMsgB, err := json.Marshal(ingressMsg)
				if err != nil {
					a.logger.Error("error marshalling ingress message", slog.String("err", err.Error()))
					return
				}
				err = a.nc.Publish(a.ingressReportingSubject, ingressMsgB)
				if err != nil {
					a.logger.Error("error publishing to update_caddy", slog.String("err", err.Error()))
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
		splitSub := strings.SplitN(r.Subject(), ".", 6)
		workloadId := splitSub[5]

		ret := models.StopWorkloadResponse{
			Id:           workloadId,
			WorkloadType: a.registerType,
			Stopped:      true,
			Message:      "",
		}

		req := new(models.StopWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			a.logger.Error("error unmarshalling stop workload request", slog.String("err", err.Error()))
			ret.Stopped = false
			ret.Message = string(models.GenericErrorsWorkloadNotFound)
			handlerError(a.logger, r, err, "100", ret)
			return
		}

		err = a.agent.StopWorkload(workloadId, req)
		if err != nil {
			a.logger.Debug("failed to stop workload", slog.String("err", err.Error()))
			ret.Stopped = false
			ret.Message = string(models.GenericErrorsWorkloadNotFound)
			handlerError(a.logger, r, err, "100", ret)
			return
		}

		err = r.RespondJSON(ret)
		if err != nil {
			a.logger.Error("error responding to stop workload request", slog.String("err", err.Error()))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.String("err", err.Error()))
			}
		}

		err = a.UnregisterTrigger(workloadId)
		if err != nil {
			a.logger.Error("error unsubscribing trigger", slog.String("err", err.Error()))
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
				a.logger.Error("error marshalling ingress message", slog.String("err", err.Error()))
				return
			}
			err = a.nc.Publish(a.ingressReportingSubject, ingressMsgB)
			if err != nil {
				a.logger.Error("error publishing to update_caddy", slog.String("err", err.Error()))
			}

		}
	}
}

func (a *Runner) handleGetWorkload() func(r micro.Request) {
	return func(r micro.Request) {
		splitSub := strings.SplitN(r.Subject(), ".", 6)
		workloadId := splitSub[5]

		// TODO: implement message with xkey
		startRequest, err := a.agent.GetWorkload(workloadId, "")
		if err != nil && err.Error() == "workload not found" {
			return
		}
		if err != nil {
			a.logger.Error("error getting workload", slog.String("err", err.Error()))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to get workload request", slog.String("err", err.Error()))
			}
			return
		}

		err = r.RespondJSON(startRequest)
		if err != nil {
			a.logger.Error("error responding to get workload request", slog.String("err", err.Error()))
		}
	}
}

func (a *Runner) handleQueryWorkloads() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.agent.<nodeid>.QUERYWORKLOADS
		req := new(models.AgentListWorkloadsRequest)

		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			a.logger.Error("error unmarshalling query workloads request", slog.String("err", err.Error()))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.String("err", err.Error()))
			}
		}

		wl, err := a.agent.QueryWorkloads(req.Namespace, req.Filter)
		if err != nil {
			a.logger.Error("error querying workloads", slog.String("err", err.Error()))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.String("err", err.Error()))
			}
		}

		err = r.RespondJSON(wl)
		if err != nil {
			a.logger.Error("error responding to query workloads request", slog.String("err", err.Error()))
		}
	}
}

func (a *Runner) handleSetLameduck() func(micro.Request) {
	return func(r micro.Request) {
		req := new(models.LameduckRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			a.logger.Error("error unmarshalling set lameduck request", slog.String("err", err.Error()))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.String("err", err.Error()))
			}
			err = r.RespondJSON(models.LameduckResponse{Success: false})
			if err != nil {
				a.logger.Error("error responding to set lameduck request", slog.String("err", err.Error()))
				err = r.Error("100", err.Error(), nil)
				if err != nil {
					a.logger.Error("error responding to start workload request", slog.String("err", err.Error()))
				}
			}
			return
		}

		delay, err := time.ParseDuration(req.Delay)
		if err != nil {
			a.logger.Error("error parsing delay", slog.String("err", err.Error()))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.String("err", err.Error()))
			}
			err = r.RespondJSON(models.LameduckResponse{Success: false})
			if err != nil {
				a.logger.Error("error responding to set lameduck request", slog.String("err", err.Error()))
				err = r.Error("100", err.Error(), nil)
				if err != nil {
					a.logger.Error("error responding to start workload request", slog.String("err", err.Error()))
				}
			}
			return
		}

		err = a.agent.SetLameduck(delay)
		if err != nil {
			a.logger.Error("error setting lameduck", slog.String("err", err.Error()))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.String("err", err.Error()))
			}
			err = r.RespondJSON(models.LameduckResponse{Success: false})
			if err != nil {
				a.logger.Error("error responding to set lameduck request", slog.String("err", err.Error()))
				err = r.Error("100", err.Error(), nil)
				if err != nil {
					a.logger.Error("error responding to start workload request", slog.String("err", err.Error()))
				}
			}
			return
		}

		err = r.RespondJSON(models.LameduckResponse{Success: true})
		if err != nil {
			a.logger.Error("error responding to set lameduck request", slog.String("err", err.Error()))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.String("err", err.Error()))
			}
		}
	}
}

func (a *Runner) handlePing() func(micro.Request) {
	return func(r micro.Request) {
		resp, err := a.agent.Ping()
		if err != nil {
			a.logger.Error("error pinging agent", slog.String("err", err.Error()))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.String("err", err.Error()))
			}
		}

		h := make(map[string][]string)
		h["agentId"] = []string{a.agentId}

		err = r.RespondJSON(resp, micro.WithHeaders(micro.Headers(h)))
		if err != nil {
			a.logger.Error("error responding to ping agent request", slog.String("err", err.Error()))
			err = r.Error("100", err.Error(), nil)
			if err != nil {
				a.logger.Error("error responding to start workload request", slog.String("err", err.Error()))
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
			a.logger.Error("error responding to workload ping", slog.String("err", err.Error()))
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

func handlerError[T models.StopWorkloadResponse | models.StartWorkloadResponse](logger *slog.Logger, r micro.Request, e error, code string, payload T) {
	logger.Error("error handling micro request", slog.String("err", e.Error()), slog.String("code", code))

	payload_b, err := json.Marshal(payload)
	if err != nil {
		logger.Error("error marshalling payload", slog.String("err", err.Error()))
	}

	err = r.Error(code, e.Error(), payload_b)
	if err != nil {
		logger.Error("failed to send micro request error message", slog.String("err", err.Error()))
	}
}
