package nexnode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	controlapi "github.com/synadia-io/nex/internal/control-api"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	EventSubjectPrefix      = "$NEX.events"
	LogSubjectPrefix        = "$NEX.logs"
	WorkloadCacheBucketName = "NEXCACHE"

	defaultHandshakeTimeoutMillis = 5000

	nexRuntimeNs = "x-nex-runtime-ns"
)

// The workload manager provides the high level strategy for the Nex node's workload management. It is responsible
// for using a process manager interface to manage processes and maintaining agent clients that communicate with
// those processes. The workload manager does not know how the agent processes are created, only how to communicate
// with them via the internal NATS server
type WorkloadManager struct {
	closing    uint32
	config     *NodeConfiguration
	kp         nkeys.KeyPair
	log        *slog.Logger
	nc         *nats.Conn
	ncInternal *nats.Conn
	cancel     context.CancelFunc
	ctx        context.Context
	t          *Telemetry

	procMan ProcessManager

	// Any agent client in this map is one that has successfully acknowledged a deployment
	activeAgents map[string]*agentapi.AgentClient
	// Agent clients in this slice are attached to processes that have not yet received a deployment AND have
	// successfully performed a handshake. Handshake failures are immediately removed
	pendingAgents map[string]*agentapi.AgentClient

	handshakes       map[string]string
	handshakeTimeout time.Duration // TODO: make configurable...

	hostServices *HostServices

	stopMutex map[string]*sync.Mutex

	// Subscriptions created on behalf of functions that cannot subscribe internallly
	triggerSubz map[string][]*nats.Subscription

	natsStoreDir string
	publicKey    string
}

// Initialize a new workload manager instance to manage and communicate with agents
func NewWorkloadManager(ctx context.Context,
	cancel context.CancelFunc,
	nodeKeypair nkeys.KeyPair,
	publicKey string,
	nc, ncint *nats.Conn,
	config *NodeConfiguration,
	log *slog.Logger,
	telemetry *Telemetry,
	procMan ProcessManager,
) (*WorkloadManager, error) {
	// Validate the node config
	if !config.Validate() {
		return nil, fmt.Errorf("failed to create new workload manager; invalid node config; %v", config.Errors)
	}

	m := &WorkloadManager{
		config:           config,
		cancel:           cancel,
		ctx:              ctx,
		handshakes:       make(map[string]string),
		handshakeTimeout: time.Duration(defaultHandshakeTimeoutMillis * time.Millisecond),
		kp:               nodeKeypair,
		log:              log,
		natsStoreDir:     defaultNatsStoreDir,
		nc:               nc,
		ncInternal:       ncint,
		publicKey:        publicKey,
		t:                telemetry,
		procMan:          procMan,

		pendingAgents: make(map[string]*agentapi.AgentClient),
		activeAgents:  make(map[string]*agentapi.AgentClient),

		stopMutex:   make(map[string]*sync.Mutex),
		triggerSubz: make(map[string][]*nats.Subscription),
	}

	m.hostServices = NewHostServices(m, nc, ncint, m.log)
	err := m.hostServices.init()
	if err != nil {
		m.log.Warn("Failed to initialize host services.", slog.Any("err", err))
		return nil, err
	}

	return m, nil
}

// Start the workload manager, which in turn starts the process manager
func (w *WorkloadManager) Start() {
	w.log.Info("Workload manager starting")

	w.procMan.Start(w)
}

// Called by the process manager when a new agent process is ready to receive deployment instructions
func (w *WorkloadManager) OnProcessReady(workloadId string) {
	agentClient := agentapi.NewAgentClient(w.ncInternal,
		2*time.Second,
		w.agentHandshakeTimedOut,
		w.agentHandshakeSucceeded,
		w.agentEvent,
		w.agentLog,
		w.log,
	)

	err := agentClient.Start(workloadId)

	if err != nil {
		w.log.Error("Failed to start agent client", slog.Any("err", err))
		return
	}

	w.pendingAgents[workloadId] = agentClient
}

// Deploys a workload to an agent by using the agent client to submit a workload deployment
// request
func (w *WorkloadManager) DeployWorkload(request *agentapi.DeployRequest) (string, error) {
	if len(w.pendingAgents) == 0 {
		return "", errors.New("no available agent client in pool, cannot deploy workload")
	}

	agentClient := w.selectRandomAgent()

	status := w.ncInternal.Status()
	workloadId := agentClient.Id()
	err := w.procMan.PrepareWorkload(workloadId, request)
	if err != nil {
		return "", fmt.Errorf("failed to prepare process for workload deployment: %s", err)
	}

	w.log.Debug("Workload manager deploying workload",
		slog.String("workloadId", workloadId),
		slog.String("conn_status", status.String()))

	deployResponse, err := agentClient.DeployWorkload(request)
	if err != nil {
		return "", fmt.Errorf("failed to submit request for workload deployment: %s", err)
	}

	if deployResponse.Accepted {
		// move the client from active to pending
		w.activeAgents[workloadId] = agentClient
		delete(w.pendingAgents, workloadId)

		if request.SupportsTriggerSubjects() {
			for _, tsub := range request.TriggerSubjects {
				sub, err := w.nc.Subscribe(tsub, w.generateTriggerHandler(workloadId, tsub, request))
				if err != nil {
					w.log.Error("Failed to create trigger subject subscription for deployed workload",
						slog.String("workloadId", workloadId),
						slog.String("trigger_subject", tsub),
						slog.String("workload_type", *request.WorkloadType),
						slog.Any("err", err),
					)
					_ = w.StopWorkload(workloadId, true)
					return "", err
				}

				w.log.Info("Created trigger subject subscription for deployed workload",
					slog.String("workloadId", workloadId),
					slog.String("trigger_subject", tsub),
					slog.String("workload_type", *request.WorkloadType),
				)

				w.triggerSubz[workloadId] = append(w.triggerSubz[workloadId], sub)
			}
		}
	} else {
		_ = w.StopWorkload(workloadId, false)
		return "", fmt.Errorf("workload rejected by agent: %s", *deployResponse.Message)
	}

	w.t.workloadCounter.Add(w.ctx, 1, metric.WithAttributes(attribute.String("workload_type", *request.WorkloadType)))
	w.t.workloadCounter.Add(w.ctx, 1, metric.WithAttributes(attribute.String("namespace", *request.Namespace)), metric.WithAttributes(attribute.String("workload_type", *request.WorkloadType)))
	w.t.deployedByteCounter.Add(w.ctx, request.TotalBytes)
	w.t.deployedByteCounter.Add(w.ctx, request.TotalBytes, metric.WithAttributes(attribute.String("namespace", *request.Namespace)))

	return workloadId, nil
}

// Stops the machine manager, which will in turn stop all firecracker VMs and attempt to clean
// up any applicable resources. Note that all "stopped" events emitted during a stop are best-effort
// and not guaranteed.
func (w *WorkloadManager) Stop() error {
	if atomic.AddUint32(&w.closing, 1) == 1 {
		w.log.Info("Workload manager stopping")

		err := w.procMan.Stop()
		if err != nil {
			w.log.Error("failed to stop process manager", slog.Any("error", err))
		}
	}

	return nil
}

// Locates a given workload by its workload ID and returns the deployment request associated with it
// Note that this means "pending" workloads are not considered by lookups
func (w *WorkloadManager) LookupWorkload(workloadId string) (*agentapi.DeployRequest, error) {
	return w.procMan.Lookup(workloadId)
}

// Retrieve a list of deployed, running workloads
func (w *WorkloadManager) RunningWorkloads() ([]controlapi.MachineSummary, error) {
	procs, err := w.procMan.ListProcesses()
	if err != nil {
		return nil, err
	}
	summaries := make([]controlapi.MachineSummary, len(procs))
	for i, p := range procs {
		summaries[i] = controlapi.MachineSummary{
			Id:        p.ID,
			Healthy:   true,
			Uptime:    "TBD",
			Namespace: p.Namespace,
			Workload: controlapi.WorkloadSummary{
				Name:         p.Name,
				Description:  *p.OriginalRequest.WorkloadName,
				Runtime:      "TBD", // TODO: replace with function exec time OR service uptime
				WorkloadType: *p.OriginalRequest.WorkloadType,
				Hash:         p.OriginalRequest.Hash,
			},
		}
	}

	return summaries, nil
}

// Stops the given workload, optionally attempting a graceful undeploy prior to
// termination
func (w *WorkloadManager) StopWorkload(workloadId string, undeploy bool) error {
	deployRequest, err := w.procMan.Lookup(workloadId)
	if err != nil {
		w.log.Warn("request to undeploy workload failed", slog.String("workloadId", workloadId), slog.String("error", err.Error()))
		return nil
	}

	if deployRequest != nil && undeploy {
		agentClient := w.activeAgents[workloadId]
		err := agentClient.Undeploy()
		if err != nil {
			w.log.Warn("request to undeploy workload via internal NATS connection failed", slog.String("workloadId", workloadId),
				slog.String("error", err.Error()))
		}
	}
	delete(w.activeAgents, workloadId)

	err = w.procMan.StopProcess(workloadId)
	if err != nil {
		w.log.Warn("failed to stop workload process", slog.String("workloadId", workloadId), slog.String("error", err.Error()))
		return err
	}
	_ = w.publishWorkloadStopped(workloadId)

	return nil
}

func (m *WorkloadManager) agentHandshakeTimedOut(workloadId string) {
	m.log.Error("Did not receive NATS handshake from agent within timeout.", slog.String("workloadId", workloadId))
	delete(m.pendingAgents, workloadId)
	if len(m.handshakes) == 0 {
		m.log.Error("First handshake failed, shutting down to avoid inconsistent behavior")
		m.cancel()
	}
}

func (m *WorkloadManager) agentHandshakeSucceeded(workloadId string) {
	now := time.Now().UTC()
	m.handshakes[workloadId] = now.Format(time.RFC3339)
}

// Generate a NATS subscriber function that is used to trigger function-type workloads
func (w *WorkloadManager) generateTriggerHandler(workloadId string, tsub string, request *agentapi.DeployRequest) func(msg *nats.Msg) {
	agentClient, ok := w.activeAgents[workloadId]
	if !ok {
		w.log.Error("Tried to generate trigger handler for non-existent agent client")
		return nil
	}
	return func(msg *nats.Msg) {

		ctx, parentSpan := tracer.Start(
			w.ctx,
			"workload-trigger",
			trace.WithNewRoot(),
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("name", *request.WorkloadName),
				attribute.String("namespace", *request.Namespace),
				attribute.String("trigger-subject", msg.Subject),
			))

		defer parentSpan.End()

		resp, err := agentClient.RunTrigger(ctx, tracer, msg.Subject, msg.Data)

		//for reference - this is what agent exec would do
		//ctx = otel.GetTextMapPropagator().Extract(cctx, propagation.HeaderCarrier(msg.Header))

		parentSpan.AddEvent("Completed internal request")
		if err != nil {
			parentSpan.SetStatus(codes.Error, "Internal trigger request failed")
			parentSpan.RecordError(err)
			w.log.Error("Failed to request agent execution via internal trigger subject",
				slog.Any("err", err),
				slog.String("trigger_subject", tsub),
				slog.String("workload_type", *request.WorkloadType),
				slog.String("workloadId", workloadId),
			)

			w.t.functionFailedTriggers.Add(w.ctx, 1)
			w.t.functionFailedTriggers.Add(w.ctx, 1, metric.WithAttributes(attribute.String("namespace", *request.Namespace)))
			w.t.functionFailedTriggers.Add(w.ctx, 1, metric.WithAttributes(attribute.String("workload_name", *request.WorkloadName)))
			_ = w.publishFunctionExecFailed(workloadId, *request.WorkloadName, tsub, err)
		} else if resp != nil {
			parentSpan.SetStatus(codes.Ok, "Trigger succeeded")
			runtimeNs := resp.Header.Get(nexRuntimeNs)
			w.log.Debug("Received response from execution via trigger subject",
				slog.String("workloadId", workloadId),
				slog.String("trigger_subject", tsub),
				slog.String("workload_type", *request.WorkloadType),
				slog.String("function_run_time_nanosec", runtimeNs),
				slog.Int("payload_size", len(resp.Data)),
			)

			runTimeNs64, err := strconv.ParseInt(runtimeNs, 10, 64)
			if err != nil {
				w.log.Warn("failed to log function runtime", slog.Any("err", err))
			}
			_ = w.publishFunctionExecSucceeded(workloadId, tsub, runTimeNs64)
			parentSpan.AddEvent("published success event")

			w.t.functionTriggers.Add(w.ctx, 1)
			w.t.functionTriggers.Add(w.ctx, 1, metric.WithAttributes(attribute.String("namespace", *request.Namespace)))
			w.t.functionTriggers.Add(w.ctx, 1, metric.WithAttributes(attribute.String("workload_name", *request.WorkloadName)))
			w.t.functionRunTimeNano.Add(w.ctx, runTimeNs64)
			w.t.functionRunTimeNano.Add(w.ctx, runTimeNs64, metric.WithAttributes(attribute.String("namespace", *request.Namespace)))
			w.t.functionRunTimeNano.Add(w.ctx, runTimeNs64, metric.WithAttributes(attribute.String("workload_name", *request.WorkloadName)))

			err = msg.Respond(resp.Data)

			if err != nil {
				parentSpan.SetStatus(codes.Error, "Failed to respond to trigger subject")
				parentSpan.RecordError(err)
				w.log.Error("Failed to respond to trigger subject subscription request for deployed workload",
					slog.String("workloadId", workloadId),
					slog.String("trigger_subject", tsub),
					slog.String("workload_type", *request.WorkloadType),
					slog.Any("err", err),
				)
			}
		}
	}
}

// Picks a pending agent from the pool that will receive the next deployment
func (w *WorkloadManager) selectRandomAgent() *agentapi.AgentClient {
	if len(w.pendingAgents) == 0 {
		return nil
	}

	// there might be a slightly faster version of this, but this effectively
	// gives us a random pick among the map elements
	for _, v := range w.pendingAgents {
		return v
	}

	return nil
}
