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
	"github.com/synadia-io/nex/models"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
)

const (
	EnvVarPrefix = "NEX_AGENT"
)

var (
	defaultEmitEvent func(string, any) error = func(string, any) error { return nil }
	defaultLogger                            = slog.New(slog.NewTextHandler(io.Discard, nil))
)

type Runner struct {
	ctx          context.Context
	name         string
	registerType string // this is used in the --type field for workloads
	version      string
	logger       *slog.Logger
	metrics      bool
	metricsPort  int

	nodeID   string
	nexus    string
	agentID  string
	triggers map[string]*triggerResources

	agent Agent
	nc    *nats.Conn
	micro micro.Service

	secretStore models.SecretStore

	// Ingress Settings
	ingressHostMachineIPAddr string

	EmitEvent func(string, any) error
}

type triggerResources struct {
	nc  *nats.Conn
	sub *nats.Subscription
}

type RunnerOpt func(*Runner) error

func WithLogger(logger *slog.Logger) RunnerOpt {
	return func(a *Runner) error {
		a.logger = logger
		return nil
	}
}

// WithSecretStore gives the agent access to pull secrets
// from a pre-configured secret store.
func WithSecretStore(secretStore models.SecretStore) RunnerOpt {
	return func(a *Runner) error {
		a.secretStore = secretStore
		return nil
	}
}

// WithPrometheusMetrics enables the prometheus metrics endpoint
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

func WithIngressSettings(hostMachineIPAddr string) RunnerOpt {
	return func(a *Runner) error {
		a.ingressHostMachineIPAddr = hostMachineIPAddr
		return nil
	}
}

func RemoteAgentInit(nc *nats.Conn, nexus, pubKey string) (*models.RegisterRemoteAgentResponse, error) {
	req := models.RegisterRemoteAgentRequest{}

	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal remote agent registration request: %w", err)
	}

	regResp, err := nc.Request(models.AgentAPIInitRemoteRegisterRequestSubject(nexus), reqB, time.Second*3)
	if err != nil {
		return nil, fmt.Errorf("failed to send remote agent registration request: %w", err)
	}

	var resp models.RegisterRemoteAgentResponse
	err = json.Unmarshal(regResp.Data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal remote agent registration response: %w", err)
	}

	return &resp, nil
}

func NewRunner(ctx context.Context, nexus, nodeID string, na Agent, opts ...RunnerOpt) (*Runner, error) {
	a := &Runner{
		ctx:          ctx,
		name:         "default",
		registerType: "default",
		version:      "0.0.0",
		logger:       defaultLogger,
		metrics:      false,
		metricsPort:  9095,

		nodeID:   nodeID,
		nexus:    nexus,
		agent:    na,
		agentID:  "default",
		triggers: make(map[string]*triggerResources),

		secretStore: nil,
		EmitEvent:   defaultEmitEvent,
	}

	for _, opt := range opts {
		if err := opt(a); err != nil {
			return nil, fmt.Errorf("failed to apply runner option: %w", err)
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

func (a *Runner) GetLogger(workloadID, namespace string, lType models.LogOut) io.Writer {
	return NewAgentLogCapture(a.nc, slog.Default(), lType, a.agentID, workloadID, namespace)
}

func (a *Runner) Run(agentID string, connData models.NatsConnectionData, eventEmitter models.EventEmitter) error {
	if eventEmitter == nil {
		return errors.New("event emitter cannot be nil")
	}
	a.EmitEvent = eventEmitter.EmitEvent

	if a.metrics {
		go func() {
			http.Handle("/metrics", promhttp.Handler())
			err := http.ListenAndServe(fmt.Sprintf(":%d", a.metricsPort), nil)
			if err != nil {
				a.logger.Error("failed to start metrics server", slog.String("err", err.Error()), slog.String("agent_id", agentID))
			}
		}()
	}

	var err error
	a.agentID = agentID

	a.nc, err = configureNatsConnection(connData)
	if err != nil {
		return fmt.Errorf("runner failed to configure initial NATS connection: %w", err)
	}

	// Register the agent
	register, err := a.agent.Register()
	if err != nil {
		return fmt.Errorf("failed agent registration: %w", err)
	}

	a.name = register.Name
	a.registerType = register.RegisterType
	a.version = register.Version

	registerB, err := json.Marshal(register)
	if err != nil {
		return fmt.Errorf("failed to marshal registration request: %w", err)
	}

	var regRet *nats.Msg

	regRet, err = a.nc.Request(models.AgentAPIRegisterRequestSubject(agentID, a.nodeID), registerB, time.Minute)
	if err != nil {
		return fmt.Errorf("failed to send agent registration request to node %s: %w", a.nodeID, err)
	}

	var regRetJSON models.RegisterAgentResponse
	err = json.Unmarshal(regRet.Data, &regRetJSON)
	if err != nil {
		return fmt.Errorf("failed to unmarshal agent registration response: %w", err)
	}

	a.nc.Close()

	if !regRetJSON.Success {
		return errors.New("agent registration failed: " + regRetJSON.Message)
	}

	a.nc, err = configureNatsConnection(regRetJSON.ConnectionData)
	if err != nil {
		return fmt.Errorf("failed to configure NATS connection with registration response data: %w", err)
	}

	// HB once immediately to ensure the agent is registered
	a.performHeartbeat()
	// Start agent heartbeat
	go func() {
		for range time.Tick(time.Second * 10) {
			a.performHeartbeat()
		}
	}()

	a.micro, err = micro.AddService(a.nc, micro.Config{
		Name:    a.name,
		Version: a.version,
	})
	if err != nil {
		return fmt.Errorf("failed to create agent micro service: %w", err)
	}

	type endpoint struct {
		Name    string
		Subject string
		Handler micro.HandlerFunc
	}

	// Start subscriptions to the host
	endpoints := []endpoint{
		{Name: "StartWorkload", Subject: models.AgentAPIStartWorkloadSubscribeSubject(a.nodeID, a.agentID), Handler: a.handleStartWorkload()},
		{Name: "StopWorkload", Subject: models.AgentAPIStopWorkloadSubscribeSubject(a.nodeID), Handler: a.handleStopWorkload()},
		{Name: "GetWorkload", Subject: models.AgentAPIGetWorkloadSubscribeSubject(a.nodeID), Handler: a.handleGetWorkload()},
		{Name: "QueryWorkloads", Subject: models.AgentAPIQueryWorkloadsSubject(a.nodeID), Handler: a.handleQueryWorkloads()},
		// System only endpoints
		{Name: "PingAgent", Subject: models.AgentAPIPingSubject(a.nodeID, a.agentID), Handler: a.handlePing()},
		{Name: "PingAllAgents", Subject: models.AgentAPIPingAllSubject(a.nodeID), Handler: a.handlePing()},
		{Name: "SetLameduck", Subject: models.AgentAPISetLameduckSubject(a.nodeID), Handler: a.handleSetLameduck()},
	}

	if _, ok := a.agent.(AgentIngessWorkloads); ok {
		endpoints = append(endpoints, endpoint{Name: "WorkloadDiscovery", Subject: models.AgentAPIPingWorkloadSubscribeSubject(a.nexus), Handler: a.handleDiscoverWorkload()})
	}

	if _, ok := a.agent.(AgentEventListener); ok {
		endpoints = append(endpoints, endpoint{Name: "EventListener", Subject: models.EventAPIPrefix(a.agentID), Handler: a.handleReceivedEvent()})
	}

	var errs error
	for _, ep := range endpoints {
		aerr := a.micro.AddEndpoint(ep.Name, ep.Handler, micro.WithEndpointSubject(ep.Subject), micro.WithEndpointQueueGroup(agentID))
		if aerr != nil {
			a.logger.Error("error adding micro endpoint", slog.String("agent_id", agentID), slog.String("endpoint_name", ep.Name), slog.String("endpoint_subject", ep.Subject), slog.String("err", aerr.Error()))
			errs = errors.Join(errs, fmt.Errorf("failed to add micro endpoint %s: %w ", ep.Name, aerr))
		}
	}
	if errs != nil {
		return fmt.Errorf("failed to add agent micro endpoints: %w", err)
	}

	if len(regRetJSON.ExistingState) > 0 {
		a.logger.Info("restoring existing state", slog.Int("num_workloads", len(regRetJSON.ExistingState)))
		go func(eState models.RegisterAgentResponseExistingState) {
			for workloadID, startRequest := range eState {
				_, err = a.agent.StartWorkload(workloadID, &startRequest, true)
				if err != nil {
					a.logger.Error("error restoring existing state", slog.String("workload_id", workloadID), slog.String("err", err.Error()))
				}
			}
		}(regRetJSON.ExistingState)
	}

	return nil
}

func (a *Runner) performHeartbeat() {
	hb, err := a.agent.Heartbeat()
	if err != nil {
		a.logger.Warn("error generating heartbeat", slog.String("err", err.Error()))
		return
	}
	hbB, err := json.Marshal(hb)
	if err != nil {
		a.logger.Warn("error marshalling heartbeat", slog.String("err", err.Error()))
		return
	}
	err = a.nc.Publish(models.AgentAPIHeartbeatSubject(a.nodeID, a.agentID), hbB)
	if err != nil {
		a.logger.Warn("error publishing heartbeat", slog.String("err", err.Error()))
		return
	}
}

func (a *Runner) Shutdown() error {
	return a.micro.Stop()
}

func (a *Runner) GetNamespaceSecret(namespace, secretKey string) ([]byte, error) {
	if a.secretStore == nil {
		return nil, errors.New("secret store not configured")
	}

	return a.secretStore.GetSecret(namespace, secretKey)
}

func (a *Runner) RegisterTrigger(workloadID, namespace, triggerSubject string, workloadConnData *models.NatsConnectionData, tFunc func([]byte) ([]byte, error)) error {
	tr := new(triggerResources)

	var err error
	tr.nc, err = configureNatsConnection(*workloadConnData)
	if err != nil {
		return fmt.Errorf("failed to configure trigger NATS connection: %w", err)
	}

	tr.sub, err = tr.nc.Subscribe(triggerSubject, func(m *nats.Msg) {
		go func() {
			startTime := time.Now()
			ret, funcError := tFunc(m.Data)
			if funcError != nil {
				a.logger.Error("error running trigger function", slog.String("err", funcError.Error()))
			}
			if m.Reply != "" { // empty if original trigger was a publish and not request
				nHeader := nats.Header{"workload_id": []string{workloadID}, "namespace": []string{namespace}}
				if funcError != nil {
					nHeader["error"] = []string{funcError.Error()}
				}
				msg := &nats.Msg{
					Subject: m.Reply,
					Header:  nHeader,
					Data:    ret,
				}
				pubErr := tr.nc.PublishMsg(msg)
				if pubErr != nil {
					a.logger.Error("error responding to trigger", slog.String("err", pubErr.Error()))
				}
			}

			runDuration := time.Since(startTime)
			if err = a.EmitEvent(namespace, models.WorkloadTriggeredEvent{
				Duration:  float64(runDuration.Milliseconds()),
				Id:        workloadID,
				Namespace: namespace,
			}); err != nil {
				a.logger.Error("error emitting workload triggered event", slog.String("err", err.Error()))
			}
		}()
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to trigger subject %s: %w", triggerSubject, err)
	}

	a.triggers[workloadID] = tr
	return nil
}

// UnregisterTrigger is used to unregister a trigger that was registered with the workload
// If a workload is stopped via successful StopWorkloadRequest, the trigger will be unregistered automatically
// If the workload fails to start inside a nexlet, use this function to clean up any unused triggers
func (a *Runner) UnregisterTrigger(workloadID string) error {
	tr, ok := a.triggers[workloadID]
	if !ok {
		a.logger.Debug("attempted to unregister a non-existent trigger", slog.String("workload_id", workloadID))
		return nil
	}

	err := tr.sub.Unsubscribe()
	if err != nil {
		a.logger.Error("failed to unsubscribe trigger", slog.String("workload_id", workloadID), slog.String("err", err.Error()))
	}

	err = tr.nc.Drain()
	if err != nil {
		a.logger.Error("failed to drain trigger connection", slog.String("workload_id", workloadID), slog.String("err", err.Error()))
	}

	delete(a.triggers, workloadID)
	return nil
}

func (a *Runner) handleStartWorkload() func(r micro.Request) {
	return func(r micro.Request) {
		splitSub := strings.SplitN(r.Subject(), ".", 7)
		workloadID := splitSub[6]

		req := new(models.AgentStartWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			handlerError(a.logger, r, err, "100", models.StartWorkloadResponse{
				Id:   workloadID,
				Name: req.Request.Name,
			})
			return
		}

		startResp, err := a.agent.StartWorkload(workloadID, req, false)
		if err != nil {
			handlerError(a.logger, r, err, "100", models.StartWorkloadResponse{
				Id:   workloadID,
				Name: req.Request.Name,
			})
			return
		}

		err = r.RespondJSON(startResp)
		if err != nil {
			handlerError(a.logger, r, err, "100", models.StartWorkloadResponse{
				Id:   workloadID,
				Name: req.Request.Name,
			})
			return
		}

		if aiw, ok := a.agent.(AgentIngessWorkloads); ok && a.ingressHostMachineIPAddr != "" {
			ports, err := aiw.GetWorkloadExposedPorts(workloadID)
			if err != nil {
				a.logger.Error("error getting exposed ports", slog.String("err", err.Error()))
			}

			for i, port := range ports {
				ingressMsg := models.AgentIngressMsg{
					WorkloadId: workloadID,
					Upstream:   fmt.Sprintf("%s:%d", a.ingressHostMachineIPAddr, port),
					Command:    models.AgentIngressCommandsAdd,
				}
				ingressMsgB, err := json.Marshal(ingressMsg)
				if err != nil {
					a.logger.Error("error marshalling ingress message", slog.String("err", err.Error()))
					return
				}
				err = a.nc.Publish(models.AgentAPIEmitEventSubject(a.agentID, "INGRESS"), ingressMsgB)
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
		workloadID := splitSub[5]

		ret := models.StopWorkloadResponse{
			Id:           workloadID,
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

		err = a.agent.StopWorkload(workloadID, req)
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

		err = a.UnregisterTrigger(workloadID)
		if err != nil {
			a.logger.Error("error unsubscribing trigger", slog.String("err", err.Error()))
		}

		if _, ok := a.agent.(AgentIngessWorkloads); ok && a.ingressHostMachineIPAddr != "" {

			ingressMsg := models.AgentIngressMsg{
				WorkloadId: workloadID,
				Command:    models.AgentIngressCommandsRemove,
			}
			ingressMsgB, err := json.Marshal(ingressMsg)
			if err != nil {
				a.logger.Error("error marshalling ingress message", slog.String("err", err.Error()))
				return
			}
			err = a.nc.Publish(models.AgentAPIEmitEventSubject(a.agentID, "INGRESS"), ingressMsgB)
			if err != nil {
				a.logger.Error("error publishing to update_caddy", slog.String("err", err.Error()))
			}

		}
	}
}

func (a *Runner) handleGetWorkload() func(r micro.Request) {
	return func(r micro.Request) {
		splitSub := strings.SplitN(r.Subject(), ".", 6)
		workloadID := splitSub[5]

		// TODO: implement message with xkey
		startRequest, err := a.agent.GetWorkload(workloadID, "")
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
		h["agentId"] = []string{a.agentID}

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

		//$NEX.SVC.<nexus>.agent.PINGWORKLOAD.<workload_id>
		subSplit := strings.SplitN(r.Subject(), ".", 6)
		workloadID := subSplit[5]

		found := ingressAgent.PingWorkload(workloadID)
		if !found {
			return
		}

		err := r.Respond(fmt.Appendf([]byte{}, `{"node_id":"%s"}`, a.nodeID))
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
	logger.Debug("error handling micro request", slog.String("err", e.Error()), slog.String("code", code))

	payloadB, err := json.Marshal(payload)
	if err != nil {
		logger.Error("error marshalling payload", slog.String("err", err.Error()))
		payloadB = []byte{}
	}

	err = r.Error(code, e.Error(), payloadB)
	if err != nil {
		logger.Error("failed to send micro request error message", slog.String("err", err.Error()))
	}
}
