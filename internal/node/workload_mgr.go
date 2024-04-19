package nexnode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"github.com/synadia-io/nex/internal/models"
	"github.com/synadia-io/nex/internal/node/observability"
	"github.com/synadia-io/nex/internal/node/processmanager"

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
	config     *models.NodeConfiguration
	kp         nkeys.KeyPair
	log        *slog.Logger
	nc         *nats.Conn
	ncInternal *nats.Conn
	cancel     context.CancelFunc
	ctx        context.Context
	t          *observability.Telemetry

	procMan processmanager.ProcessManager

	// Any agent client in this map is one that has successfully acknowledged a deployment
	activeAgents map[string]*agentapi.AgentClient

	// Agent clients in this slice are attached to processes that have not yet received a deployment AND have
	// successfully performed a handshake. Handshake failures are immediately removed
	pendingAgents map[string]*agentapi.AgentClient

	handshakes       map[string]string
	handshakeTimeout time.Duration // TODO: make configurable...

	hostServices *HostServices

	poolMutex *sync.Mutex
	stopMutex map[string]*sync.Mutex

	// Subscriptions created on behalf of functions that cannot subscribe internallly
	subz map[string][]*nats.Subscription

	natsStoreDir string
	publicKey    string
}

// Initialize a new workload manager instance to manage and communicate with agents
func NewWorkloadManager(
	ctx context.Context,
	cancel context.CancelFunc,
	nodeKeypair nkeys.KeyPair,
	publicKey string,
	nc, ncint *nats.Conn,
	config *models.NodeConfiguration,
	log *slog.Logger,
	telemetry *observability.Telemetry,
) (*WorkloadManager, error) {
	// Validate the node config
	if !config.Validate() {
		return nil, fmt.Errorf("failed to create new workload manager; invalid node config; %v", config.Errors)
	}

	w := &WorkloadManager{
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
		poolMutex:        &sync.Mutex{},
		publicKey:        publicKey,
		t:                telemetry,

		pendingAgents: make(map[string]*agentapi.AgentClient),
		activeAgents:  make(map[string]*agentapi.AgentClient),

		stopMutex: make(map[string]*sync.Mutex),
		subz:      make(map[string][]*nats.Subscription),
	}

	var err error

	w.procMan, err = processmanager.NewProcessManager(w.log, w.config, w.t, w.ctx)
	if err != nil {
		w.log.Error("Failed to initialize agent process manager", slog.Any("error", err))
		return nil, err
	}

	w.hostServices = NewHostServices(w, nc, ncint, w.log)
	err = w.hostServices.init()
	if err != nil {
		w.log.Warn("Failed to initialize host services.", slog.Any("err", err))
		return nil, err
	}

	return w, nil
}

// Start the workload manager, which in turn starts the configured agent process manager
func (w *WorkloadManager) Start() {
	w.log.Info("Workload manager starting")

	err := w.procMan.Start(w)
	if err != nil {
		w.log.Error("Agent process manager failed to start", slog.Any("error", err))
		w.cancel()
	}
}

func (m *WorkloadManager) CacheWorkload(request *controlapi.DeployRequest) (uint64, *string, error) {
	bucket := request.Location.Host
	key := strings.Trim(request.Location.Path, "/")

	m.log.Info("Attempting object store download", slog.String("bucket", bucket), slog.String("key", key), slog.String("url", m.nc.Opts.Url))

	opts := []nats.JSOpt{}
	if request.JsDomain != nil {
		opts = append(opts, nats.APIPrefix(*request.JsDomain))
	}

	js, err := m.nc.JetStream(opts...)
	if err != nil {
		return 0, nil, err
	}

	store, err := js.ObjectStore(bucket)
	if err != nil {
		m.log.Error("Failed to bind to source object store", slog.Any("err", err), slog.String("bucket", bucket))
		return 0, nil, err
	}

	_, err = store.GetInfo(key)
	if err != nil {
		m.log.Error("Failed to locate workload binary in source object store", slog.Any("err", err), slog.String("key", key), slog.String("bucket", bucket))
		return 0, nil, err
	}

	workload, err := store.GetBytes(key)
	if err != nil {
		m.log.Error("Failed to download bytes from source object store", slog.Any("err", err), slog.String("key", key))
		return 0, nil, err
	}

	jsInternal, err := m.ncInternal.JetStream()
	if err != nil {
		m.log.Error("Failed to acquire JetStream context for internal object store.", slog.Any("err", err))
		panic(err)
	}

	cache, err := jsInternal.ObjectStore(agentapi.WorkloadCacheBucket)
	if err != nil {
		m.log.Error("Failed to get object store reference for internal cache.", slog.Any("err", err))
		panic(err)
	}

	obj, err := cache.PutBytes(request.DecodedClaims.Subject, workload)
	if err != nil {
		m.log.Error("Failed to write workload to internal cache.", slog.Any("err", err))
		panic(err)
	}

	workloadHash := sha256.New()
	workloadHash.Write(workload)
	workloadHashString := hex.EncodeToString(workloadHash.Sum(nil))

	m.log.Info("Successfully stored workload in internal object store", slog.String("name", request.DecodedClaims.Subject), slog.Int64("bytes", int64(obj.Size)))
	return obj.Size, &workloadHashString, nil
}

// Deploy a workload as specified by the given deploy request to an available
// agent in the configured pool
func (w *WorkloadManager) DeployWorkload(request *agentapi.DeployRequest) (*string, error) {
	w.poolMutex.Lock()
	defer w.poolMutex.Unlock()

	agentClient, err := w.selectRandomAgent()
	if err != nil {
		return nil, fmt.Errorf("failed to deploy workload: %s", err)
	}

	workloadID := agentClient.ID()
	err = w.procMan.PrepareWorkload(workloadID, request)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare agent process for workload deployment: %s", err)
	}

	status := w.ncInternal.Status()

	w.log.Debug("Workload manager deploying workload",
		slog.String("workload_id", workloadID),
		slog.String("conn_status", status.String()))

	deployResponse, err := agentClient.DeployWorkload(request)
	if err != nil {
		return nil, fmt.Errorf("failed to submit request for workload deployment: %s", err)
	}

	if deployResponse.Accepted {
		// move the client from active to pending
		w.activeAgents[workloadID] = agentClient
		delete(w.pendingAgents, workloadID)

		if request.SupportsTriggerSubjects() {
			for _, tsub := range request.TriggerSubjects {
				sub, err := w.nc.Subscribe(tsub, w.generateTriggerHandler(workloadID, tsub, request))
				if err != nil {
					w.log.Error("Failed to create trigger subject subscription for deployed workload",
						slog.String("workload_id", workloadID),
						slog.String("trigger_subject", tsub),
						slog.String("workload_type", *request.WorkloadType),
						slog.Any("err", err),
					)
					_ = w.StopWorkload(workloadID, true)
					return nil, err
				}

				w.log.Info("Created trigger subject subscription for deployed workload",
					slog.String("workload_id", workloadID),
					slog.String("trigger_subject", tsub),
					slog.String("workload_type", *request.WorkloadType),
				)

				w.subz[workloadID] = append(w.subz[workloadID], sub)
			}
		}
	} else {
		_ = w.StopWorkload(workloadID, false)
		return nil, fmt.Errorf("workload rejected by agent: %s", *deployResponse.Message)
	}

	w.t.WorkloadCounter.Add(w.ctx, 1, metric.WithAttributes(attribute.String("workload_type", *request.WorkloadType)))
	w.t.WorkloadCounter.Add(w.ctx, 1, metric.WithAttributes(attribute.String("namespace", *request.Namespace)), metric.WithAttributes(attribute.String("workload_type", *request.WorkloadType)))
	w.t.DeployedByteCounter.Add(w.ctx, request.TotalBytes)
	w.t.DeployedByteCounter.Add(w.ctx, request.TotalBytes, metric.WithAttributes(attribute.String("namespace", *request.Namespace)))

	return &workloadID, nil
}

// Locates a given workload by its workload ID and returns the deployment request associated with it
// Note that this means "pending" workloads are not considered by lookups
func (w *WorkloadManager) LookupWorkload(workloadID string) (*agentapi.DeployRequest, error) {
	return w.procMan.Lookup(workloadID)
}

// Retrieve a list of deployed, running workloads
func (w *WorkloadManager) RunningWorkloads() ([]controlapi.MachineSummary, error) {
	procs, err := w.procMan.ListProcesses()
	if err != nil {
		return nil, err
	}

	summaries := make([]controlapi.MachineSummary, len(procs))

	for i, p := range procs {
		uptimeFriendly := "unknown"
		runtimeFriendly := "unknown"
		agentClient, ok := w.activeAgents[p.ID]
		if ok {
			uptimeFriendly = myUptime(agentClient.UptimeMillis())
			if *p.DeployRequest.WorkloadType == "v8" || *p.DeployRequest.WorkloadType == "wasm" {
				nanoTime := fmt.Sprintf("%dns", agentClient.ExecTimeNanos())
				rt, err := time.ParseDuration(nanoTime)
				if err == nil {
					if rt.Nanoseconds() < 1000 {
						runtimeFriendly = nanoTime
					} else if rt.Milliseconds() < 1000 {
						runtimeFriendly = fmt.Sprintf("%dms", rt.Milliseconds())
					} else {
						runtimeFriendly = myUptime(rt)
					}
				} else {
					w.log.Warn("Failed to generate parsed time from nanos", slog.Any("error", err))
				}
			} else {
				runtimeFriendly = uptimeFriendly
			}
		}

		summaries[i] = controlapi.MachineSummary{
			Id:        p.ID,
			Healthy:   true,
			Uptime:    uptimeFriendly,
			Namespace: p.Namespace,
			Workload: controlapi.WorkloadSummary{
				Name:         p.Name,
				Description:  *p.DeployRequest.Description,
				Runtime:      runtimeFriendly,
				WorkloadType: *p.DeployRequest.WorkloadType,
				Hash:         p.DeployRequest.Hash,
			},
		}
	}

	return summaries, nil
}

// Stop the workload manager, which will in turn stop all managed agents and attempt to clean
// up all applicable resources.
func (w *WorkloadManager) Stop() error {
	if atomic.AddUint32(&w.closing, 1) == 1 {
		w.log.Info("Workload manager stopping")

		err := w.procMan.Stop()
		if err != nil {
			w.log.Error("failed to stop agent process manager", slog.Any("error", err))
			return err
		}
	}

	return nil
}

// Stop a workload, optionally attempting a graceful undeploy prior to termination
func (w *WorkloadManager) StopWorkload(id string, undeploy bool) error {
	deployRequest, err := w.procMan.Lookup(id)
	if err != nil {
		w.log.Warn("request to undeploy workload failed", slog.String("workload_id", id), slog.String("error", err.Error()))
		return err
	}

	mutex := w.stopMutex[id]
	mutex.Lock()
	defer mutex.Unlock()

	w.log.Debug("Attempting to stop workload", slog.String("workload_id", id), slog.Bool("undeploy", undeploy))

	for _, sub := range w.subz[id] {
		err := sub.Drain()
		if err != nil {
			w.log.Warn(fmt.Sprintf("failed to drain subscription to subject %s associated with workload %s: %s", sub.Subject, id, err.Error()))
		}

		w.log.Debug(fmt.Sprintf("drained subscription to subject %s associated with workload %s", sub.Subject, id))
	}

	if deployRequest != nil && undeploy {
		agentClient := w.activeAgents[id]
		defer func() {
			_ = agentClient.Drain()
		}()

		err := agentClient.Undeploy()
		if err != nil {
			w.log.Warn("request to undeploy workload via internal NATS connection failed", slog.String("workload_id", id), slog.String("error", err.Error()))
		}
	}

	err = w.procMan.StopProcess(id)
	if err != nil {
		w.log.Warn("failed to stop workload process", slog.String("workload_id", id), slog.String("error", err.Error()))
		return err
	}

	delete(w.activeAgents, id)
	delete(w.stopMutex, id)

	_ = w.publishWorkloadStopped(id)

	return nil
}

// Called by the agent process manager when an agent has been warmed and is ready
// to receive workload deployment instructions
func (w *WorkloadManager) OnProcessStarted(id string) {
	w.log.Debug("Process started", slog.String("workload_id", id))
	w.poolMutex.Lock()
	defer w.poolMutex.Unlock()

	agentClient := agentapi.NewAgentClient(
		w.ncInternal,
		w.log,
		w.handshakeTimeout,
		w.agentHandshakeTimedOut,
		w.agentHandshakeSucceeded,
		w.agentEvent,
		w.agentLog,
	)

	err := agentClient.Start(id)
	if err != nil {
		w.log.Error("Failed to start agent client", slog.Any("err", err))
		return
	}

	w.pendingAgents[id] = agentClient
	w.stopMutex[id] = &sync.Mutex{}
}

func (w *WorkloadManager) agentHandshakeTimedOut(id string) {
	w.poolMutex.Lock()
	defer w.poolMutex.Unlock()

	w.log.Error("Did not receive NATS handshake from agent within timeout.", slog.String("workload_id", id))
	delete(w.pendingAgents, id)

	if len(w.handshakes) == 0 {
		w.log.Error("First handshake failed, shutting down to avoid inconsistent behavior")
		w.cancel()
	}
}

func (w *WorkloadManager) agentHandshakeSucceeded(workloadID string) {
	now := time.Now().UTC()
	w.handshakes[workloadID] = now.Format(time.RFC3339)
}

// Generate a NATS subscriber function that is used to trigger function-type workloads
func (w *WorkloadManager) generateTriggerHandler(workloadID string, tsub string, request *agentapi.DeployRequest) func(msg *nats.Msg) {
	agentClient, ok := w.activeAgents[workloadID]
	if !ok {
		w.log.Error("Attempted to generate trigger handler for non-existent agent client")
		return nil
	}

	return func(msg *nats.Msg) {
		ctx, parentSpan := w.t.Tracer.Start(
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

		resp, err := agentClient.RunTrigger(ctx, w.t.Tracer, msg.Subject, msg.Data)

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
				slog.String("workload_id", workloadID),
			)

			w.t.FunctionFailedTriggers.Add(w.ctx, 1)
			w.t.FunctionFailedTriggers.Add(w.ctx, 1, metric.WithAttributes(attribute.String("namespace", *request.Namespace)))
			w.t.FunctionFailedTriggers.Add(w.ctx, 1, metric.WithAttributes(attribute.String("workload_name", *request.WorkloadName)))
			_ = w.publishFunctionExecFailed(workloadID, *request.WorkloadName, tsub, err)
		} else if resp != nil {
			parentSpan.SetStatus(codes.Ok, "Trigger succeeded")
			runtimeNs := resp.Header.Get(nexRuntimeNs)
			w.log.Debug("Received response from execution via trigger subject",
				slog.String("workload_id", workloadID),
				slog.String("trigger_subject", tsub),
				slog.String("workload_type", *request.WorkloadType),
				slog.String("function_run_time_nanosec", runtimeNs),
				slog.Int("payload_size", len(resp.Data)),
			)

			runTimeNs64, err := strconv.ParseInt(runtimeNs, 10, 64)
			if err != nil {
				w.log.Warn("failed to log function runtime", slog.Any("err", err))
			}
			_ = w.publishFunctionExecSucceeded(workloadID, tsub, runTimeNs64)
			agentClient.RecordExecTime(runTimeNs64)
			parentSpan.AddEvent("published success event")

			w.t.FunctionTriggers.Add(w.ctx, 1)
			w.t.FunctionTriggers.Add(w.ctx, 1, metric.WithAttributes(attribute.String("namespace", *request.Namespace)))
			w.t.FunctionTriggers.Add(w.ctx, 1, metric.WithAttributes(attribute.String("workload_name", *request.WorkloadName)))
			w.t.FunctionRunTimeNano.Add(w.ctx, runTimeNs64)
			w.t.FunctionRunTimeNano.Add(w.ctx, runTimeNs64, metric.WithAttributes(attribute.String("namespace", *request.Namespace)))
			w.t.FunctionRunTimeNano.Add(w.ctx, runTimeNs64, metric.WithAttributes(attribute.String("workload_name", *request.WorkloadName)))

			err = msg.Respond(resp.Data)

			if err != nil {
				parentSpan.SetStatus(codes.Error, "Failed to respond to trigger subject")
				parentSpan.RecordError(err)
				w.log.Error("Failed to respond to trigger subject subscription request for deployed workload",
					slog.String("workload_id", workloadID),
					slog.String("trigger_subject", tsub),
					slog.String("workload_type", *request.WorkloadType),
					slog.Any("err", err),
				)
			}
		}
	}
}

// Picks a pending agent from the pool that will receive the next deployment
func (w *WorkloadManager) selectRandomAgent() (*agentapi.AgentClient, error) {
	if len(w.pendingAgents) == 0 {
		return nil, errors.New("no available agent client in pool")
	}

	// there might be a slightly faster version of this, but this effectively
	// gives us a random pick among the map elements
	for _, v := range w.pendingAgents {
		return v, nil
	}

	return nil, nil
}
