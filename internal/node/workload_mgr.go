package nexnode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	internalnats "github.com/synadia-io/nex/internal/node/internal-nats"
	"github.com/synadia-io/nex/internal/node/observability"
	"github.com/synadia-io/nex/internal/node/processmanager"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultInternalNatsStoreDir = "pnats"

	EventSubjectPrefix      = "$NEX.events"
	LogSubjectPrefix        = "$NEX.logs"
	WorkloadCacheBucketName = "NEXCACHE"
)

// The workload manager provides the high level strategy for the Nex node's workload management. It is responsible
// for using a process manager interface to manage processes and maintaining agent clients that communicate with
// those processes. The workload manager does not know how the agent processes are created, only how to communicate
// with them via the internal NATS server
type WorkloadManager struct {
	closing uint32
	config  *models.NodeConfiguration
	kp      nkeys.KeyPair
	log     *slog.Logger
	cancel  context.CancelFunc
	ctx     context.Context
	t       *observability.Telemetry

	nc      *nats.Conn
	natsint *internalnats.InternalNatsServer
	ncint   *nats.Conn

	procMan processmanager.ProcessManager

	// Any agent client in this map is one that has successfully acknowledged a deployment
	liveAgents map[string]*agentapi.AgentClient

	// Agent clients in this slice are attached to processes that have not yet received a deployment AND have
	// successfully performed a handshake. Handshake failures are immediately removed
	poolAgents map[string]*agentapi.AgentClient

	handshakes       map[string]string
	handshakeTimeout time.Duration
	pingTimeout      time.Duration

	hostServices *HostServices

	poolMutex *sync.Mutex
	stopMutex map[string]*sync.Mutex

	// Subscriptions created on behalf of functions that cannot subscribe internallly
	subz map[string][]*nats.Subscription

	publicKey string
}

// Initialize a new workload manager instance to manage and communicate with agents
func NewWorkloadManager(
	ctx context.Context,
	cancel context.CancelFunc,
	nodeKeypair nkeys.KeyPair,
	publicKey string,
	nc *nats.Conn,
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
		handshakeTimeout: time.Duration(config.AgentHandshakeTimeoutMillisecond) * time.Millisecond,
		kp:               nodeKeypair,
		log:              log,
		nc:               nc,
		poolMutex:        &sync.Mutex{},
		pingTimeout:      time.Duration(config.AgentPingTimeoutMillisecond) * time.Millisecond,
		publicKey:        publicKey,
		t:                telemetry,

		poolAgents: make(map[string]*agentapi.AgentClient),
		liveAgents: make(map[string]*agentapi.AgentClient),

		stopMutex: make(map[string]*sync.Mutex),
		subz:      make(map[string][]*nats.Subscription),
	}

	var err error

	// start internal NATS server
	err = w.startInternalNATS()
	if err != nil {
		w.log.Error("Failed to start internal NATS server", slog.Any("err", err))
		return nil, err
	} else {
		w.log.Debug("Internal NATS server started", slog.String("client_url", w.natsint.ClientURL()))
	}

	w.hostServices = NewHostServices(w.ncint, config.HostServicesConfig, w.log, w.t.Tracer)
	err = w.hostServices.init()
	if err != nil {
		w.log.Warn("Failed to initialize host services", slog.Any("err", err))
		return nil, err
	}

	w.procMan, err = processmanager.NewProcessManager(w.natsint, w.log, w.config, w.t, w.ctx)
	if err != nil {
		w.log.Error("Failed to initialize agent process manager", slog.Any("error", err))
		return nil, err
	}

	return w, nil
}

// Start the workload manager, which in turn starts the configured agent process manager
func (w *WorkloadManager) Start() {
	w.log.Debug("Workload manager starting")

	err := w.procMan.Start(w)
	if err != nil {
		w.log.Error("Agent process manager failed to start", slog.Any("error", err))
		w.cancel()
	}
}

func (m *WorkloadManager) CacheWorkload(workloadID string, request *controlapi.DeployRequest) (uint64, *string, error) {
	bucket := request.Location.Host
	key := strings.Trim(request.Location.Path, "/")

	jsLogAttr := []any{slog.String("bucket", bucket), slog.String("key", key)}
	opts := []nats.JSOpt{}
	if request.JsDomain != nil {
		opts = append(opts, nats.Domain(*request.JsDomain))

		jsLogAttr = append(jsLogAttr, slog.String("jsdomain", *request.JsDomain))
	}

	m.log.Info("Attempting object store download", jsLogAttr...)

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

	started := time.Now()
	workload, err := store.GetBytes(key)
	if err != nil {
		m.log.Error("Failed to download bytes from source object store", slog.Any("err", err), slog.String("key", key))
		return 0, nil, err
	}
	finished := time.Since(started)
	dlRate := float64(len(workload)) / finished.Seconds()
	m.log.Debug("CacheWorkload object store download completed", slog.String("name", key), slog.String("duration", fmt.Sprintf("%.2f sec", finished.Seconds())), slog.String("rate", fmt.Sprintf("%s/sec", byteConvert(dlRate))))

	err = m.natsint.StoreFileForID(workloadID, workload)
	if err != nil {
		m.log.Error("Failed to store bytes from source object store in cache", slog.Any("err", err), slog.String("key", key))
	}

	workloadHash := sha256.New()
	workloadHash.Write(workload)
	workloadHashString := hex.EncodeToString(workloadHash.Sum(nil))

	m.log.Info("Successfully stored workload in internal object store",
		slog.String("workload_name", *request.WorkloadName),
		slog.String("workload_id", workloadID),
		slog.String("workload_hash", workloadHashString),
		slog.Int("bytes", len(workload)),
	)

	return uint64(len(workload)), &workloadHashString, nil
}

// Deploy a workload as specified by the given deploy request to an available
// agent in the configured pool
func (w *WorkloadManager) DeployWorkload(agentClient *agentapi.AgentClient, request *agentapi.DeployRequest) error {
	w.poolMutex.Lock()
	defer w.poolMutex.Unlock()

	if w.config.AllowDuplicateWorkloads != nil && !*w.config.AllowDuplicateWorkloads {
		for _, agentClient := range w.liveAgents {
			if strings.EqualFold(agentClient.DeployRequest().Hash, request.Hash) {
				w.log.Warn("Attempted to deploy duplicate workload",
					slog.String("workload_name", *request.WorkloadName),
					slog.String("workload_type", string(request.WorkloadType)),
				)

				return errors.New("attempted to deploy duplicate workload to node configured to reject duplicates")
			}
		}
	}

	workloadID := agentClient.ID()
	err := w.procMan.PrepareWorkload(workloadID, request)
	if err != nil {
		return fmt.Errorf("failed to prepare agent process for workload deployment: %s", err)
	}

	w.log.Debug("Attempting to deploy workload",
		slog.String("workload_id", workloadID),
		slog.String("workload_name", *request.WorkloadName),
		slog.String("workload_type", string(request.WorkloadType)),
		slog.String("status", w.ncint.Status().String()),
	)

	if w.config.HostServicesConfig != nil && request.WorkloadType == controlapi.NexWorkloadNative {
		request.Environment["NEX_HOSTSERVICES_NATS_SERVER"] = w.config.HostServicesConfig.NatsUrl
		request.Environment["NEX_HOSTSERVICES_NATS_USER_JWT"] = w.config.HostServicesConfig.NatsUserJwt
		request.Environment["NEX_HOSTSERVICES_NATS_USER_SEED"] = w.config.HostServicesConfig.NatsUserSeed
	}

	deployResponse, err := agentClient.DeployWorkload(request)
	if err != nil {
		delete(w.poolAgents, workloadID) // FIXME!!! does this leak running agents??
		return fmt.Errorf("failed to submit request for workload deployment: %s", err)
	}

	if deployResponse.Accepted {
		w.log.Debug("Workload manager deploy attempt accepted",
			slog.String("workload_id", workloadID),
		)

		// move the client from pool to pending
		w.liveAgents[workloadID] = agentClient
		delete(w.poolAgents, workloadID)

		if request.WorkloadType != controlapi.NexWorkloadNative {
			ncHostServices, err := w.createHostServicesConnection(request)
			if err != nil {
				w.log.Error("Failed to establish host services connection for workload",
					slog.Any("error", err),
				)
				return err
			}

			w.hostServices.server.SetHostServicesConnection(workloadID, ncHostServices)

			if request.SupportsTriggerSubjects() {
				for _, tsub := range request.TriggerSubjects {
					sub, err := ncHostServices.Subscribe(tsub, w.generateTriggerHandler(workloadID, tsub, request))
					if err != nil {
						w.log.Error("Failed to create trigger subject subscription for deployed workload",
							slog.String("workload_id", workloadID),
							slog.String("trigger_subject", tsub),
							slog.String("workload_type", string(request.WorkloadType)),
							slog.Any("err", err),
						)
						_ = w.StopWorkload(workloadID, true)
						return err
					}

					w.log.Debug("Created trigger subject subscription for deployed workload",
						slog.String("workload_id", workloadID),
						slog.String("nats_url", ncHostServices.ConnectedAddr()),
						slog.String("trigger_subject", tsub),
						slog.String("workload_type", string(request.WorkloadType)),
					)

					w.subz[workloadID] = append(w.subz[workloadID], sub)
				}
			}
		}
	} else {
		_ = w.StopWorkload(workloadID, false)
		return fmt.Errorf("workload rejected by agent: %s", *deployResponse.Message)
	}

	w.t.WorkloadCounter.Add(w.ctx, 1, metric.WithAttributes(attribute.String("workload_type", string(request.WorkloadType))))
	w.t.WorkloadCounter.Add(w.ctx, 1, metric.WithAttributes(attribute.String("namespace", *request.Namespace)), metric.WithAttributes(attribute.String("workload_type", string(request.WorkloadType))))
	w.t.DeployedByteCounter.Add(w.ctx, request.TotalBytes)
	w.t.DeployedByteCounter.Add(w.ctx, request.TotalBytes, metric.WithAttributes(attribute.String("namespace", *request.Namespace)))

	return nil
}

// Locates a given workload by its workload ID and returns the deployment request associated with it
// Note that this means "pending" agents are not considered by lookups
func (w *WorkloadManager) LookupWorkload(workloadID string) (*agentapi.DeployRequest, error) {
	if agentClient, ok := w.liveAgents[workloadID]; ok {
		return agentClient.DeployRequest(), nil
	}

	return nil, fmt.Errorf("workload doesn't exist: %s", workloadID)
}

// Retrieve a list of deployed, running workloads
func (w *WorkloadManager) RunningWorkloads() ([]controlapi.MachineSummary, error) {
	summaries := make([]controlapi.MachineSummary, 0)

	for id, agentClient := range w.liveAgents {
		deployRequest := agentClient.DeployRequest()

		uptimeFriendly := myUptime(agentClient.UptimeMillis())
		runtimeFriendly := "--"

		if agentClient.WorkloadType() == controlapi.NexWorkloadV8 || agentClient.WorkloadType() == controlapi.NexWorkloadWasm {
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
		}

		summaries = append(summaries, controlapi.MachineSummary{
			Id:        id,
			Healthy:   true, // FIXME!!!
			Uptime:    uptimeFriendly,
			Namespace: *deployRequest.Namespace,
			Workload: controlapi.WorkloadSummary{
				Name:         *deployRequest.WorkloadName,
				Description:  *deployRequest.Description,
				Hash:         deployRequest.Hash,
				Runtime:      runtimeFriendly,
				Uptime:       uptimeFriendly,
				WorkloadType: deployRequest.WorkloadType,
			},
		})
	}

	return summaries, nil
}

// Stop the workload manager, which will in turn stop all managed agents and attempt to clean
// up all applicable resources.
func (w *WorkloadManager) Stop() error {
	if atomic.AddUint32(&w.closing, 1) == 1 {
		for id := range w.poolAgents {
			_ = w.poolAgents[id].Stop()
		}

		for id := range w.liveAgents {
			err := w.StopWorkload(id, true)
			if err != nil {
				w.log.Warn("Failed to stop agent", slog.String("workload_id", id), slog.String("error", err.Error()))
			}
		}

		err := w.procMan.Stop()
		if err != nil {
			w.log.Error("failed to stop agent process manager", slog.Any("error", err))
			return err
		}

		_ = w.ncint.Drain()
		for !w.ncint.IsClosed() {
			time.Sleep(time.Millisecond * 25)
		}

		w.natsint.Shutdown()
		w.log.Debug("Workload manager stopped")
	}

	return nil
}

// Stop a workload, optionally attempting a graceful undeploy prior to termination
func (w *WorkloadManager) StopWorkload(id string, undeploy bool) error {
	deployRequest, err := w.LookupWorkload(id)
	if err != nil {
		w.log.Warn("request to undeploy workload failed", slog.String("workload_id", id), slog.String("error", err.Error()))
		return err
	}

	mutex := w.stopMutex[id]
	if mutex != nil {
		mutex.Lock()
		defer mutex.Unlock()
	}

	w.log.Debug("Attempting to stop workload", slog.String("workload_id", id), slog.Bool("undeploy", undeploy))

	for _, sub := range w.subz[id] {
		err := sub.Drain()
		if err != nil {
			w.log.Warn("failed to drain subscription to subject associated with workload",
				slog.String("subject", sub.Subject),
				slog.String("workload_id", id),
				slog.String("err", err.Error()),
			)
		}

		w.log.Debug("drained subscription associated with workload",
			slog.String("subject", sub.Subject),
			slog.String("workload_id", id),
		)
	}

	if deployRequest != nil && undeploy {
		agentClient := w.liveAgents[id]
		defer func() {
			_ = agentClient.Drain()
		}()

		err := agentClient.Undeploy()
		if err != nil {
			w.log.Warn("request to undeploy workload via internal NATS connection failed", slog.String("workload_id", id), slog.String("error", err.Error()))
		}

		// FIXME-- this should probably just live in workload manager
		_ = w.natsint.DestroyCredentials(id)
	}

	err = w.procMan.StopProcess(id)
	if err != nil {
		w.log.Warn("failed to stop workload process", slog.String("workload_id", id), slog.String("error", err.Error()))
		return err
	}

	_ = w.publishWorkloadUndeployed(id)

	delete(w.liveAgents, id)
	delete(w.poolAgents, id)
	delete(w.stopMutex, id)
	w.hostServices.server.RemoveHostServicesConnection(id)

	return nil
}

// Called by the agent process manager when an agent has been warmed and is ready
// to receive workload deployment instructions
func (w *WorkloadManager) OnProcessStarted(id string) {
	w.poolMutex.Lock()
	defer w.poolMutex.Unlock()

	w.log.Debug("Process started", slog.String("workload_id", id))

	clientConn, err := w.natsint.ConnectionByID(id)
	if err != nil {
		w.log.Error("Failed to resolve internal NATS connection for agent client", slog.Any("err", err))
		return
	}

	agentClient := agentapi.NewAgentClient(
		clientConn,
		w.log,
		w.handshakeTimeout,
		w.pingTimeout,
		w.agentHandshakeTimedOut,
		w.agentHandshakeSucceeded,
		w.agentContactLost,
		w.agentEvent,
		w.agentLog,
	)

	err = agentClient.Start(id)
	if err != nil {
		w.log.Error("Failed to start agent client", slog.Any("err", err))
		return
	}

	w.poolAgents[id] = agentClient
	w.stopMutex[id] = &sync.Mutex{}
}

func (w *WorkloadManager) agentHandshakeTimedOut(id string) {
	w.poolMutex.Lock()
	defer w.poolMutex.Unlock()

	w.log.Error("Did not receive NATS handshake from agent within timeout.", slog.String("workload_id", id))
	delete(w.poolAgents, id)

	if len(w.handshakes) == 0 {
		w.log.Error("First handshake failed, shutting down to avoid inconsistent behavior")
		w.cancel()
	}
}

func (w *WorkloadManager) agentHandshakeSucceeded(workloadID string) {
	w.poolMutex.Lock()
	defer w.poolMutex.Unlock()

	now := time.Now().UTC()
	w.handshakes[workloadID] = now.Format(time.RFC3339)
}

func (w *WorkloadManager) agentContactLost(workloadID string) {
	w.log.Debug("Lost contact with agent", slog.String("workload_id", workloadID))
	if _, ok := w.liveAgents[workloadID]; ok {
		_ = w.StopWorkload(workloadID, false)
	}
}

// Generate a NATS subscriber function that is used to trigger function-type workloads
func (w *WorkloadManager) generateTriggerHandler(workloadID string, tsub string, request *agentapi.DeployRequest) func(msg *nats.Msg) {
	agentClient, ok := w.liveAgents[workloadID]
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

		parentSpan.AddEvent("Completed internal request")
		if err != nil {
			parentSpan.SetStatus(codes.Error, "Internal trigger request failed")
			parentSpan.RecordError(err)
			w.log.Error("Failed to request agent execution via internal trigger subject",
				slog.Any("err", err),
				slog.String("trigger_subject", tsub),
				slog.String("workload_type", string(request.WorkloadType)),
				slog.String("workload_id", workloadID),
			)

			w.t.FunctionFailedTriggers.Add(w.ctx, 1)
			w.t.FunctionFailedTriggers.Add(w.ctx, 1, metric.WithAttributes(attribute.String("namespace", *request.Namespace)))
			w.t.FunctionFailedTriggers.Add(w.ctx, 1, metric.WithAttributes(attribute.String("workload_name", *request.WorkloadName)))
			_ = w.publishFunctionExecFailed(workloadID, tsub, err)
		} else if resp != nil {
			parentSpan.SetStatus(codes.Ok, "Trigger succeeded")
			runtimeNs := resp.Header.Get(agentapi.NexRuntimeNs)
			w.log.Debug("Received response from execution via trigger subject",
				slog.String("workload_id", workloadID),
				slog.String("trigger_subject", tsub),
				slog.String("workload_type", string(request.WorkloadType)),
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
					slog.String("workload_type", string(request.WorkloadType)),
					slog.Any("err", err),
				)
			}
		}
	}
}

func (w *WorkloadManager) startInternalNATS() error {
	storeDir := filepath.Join(os.TempDir(), defaultInternalNatsStoreDir)
	if w.config.InternalNodeStoreDir != nil {
		storeDir = *w.config.InternalNodeStoreDir
	}

	var err error
	w.natsint, err = internalnats.NewInternalNatsServer(w.log, storeDir, w.config.InternalNodeDebug, w.config.InternalNodeTrace)
	if err != nil {
		return err
	}

	p := w.natsint.Port()
	w.config.InternalNodePort = &p
	w.ncint = w.natsint.Connection()

	return nil
}

func (w *WorkloadManager) createHostServicesConnection(request *agentapi.DeployRequest) (*nats.Conn, error) {
	natsOpts := []nats.Option{
		nats.Name("nex-hostservices"),
	}

	var url string
	if request.HostServicesConfig != nil {
		// FIXME-- check to ensure NATS user JWT and seed are present
		natsOpts = append(natsOpts,
			nats.UserJWTAndSeed(request.HostServicesConfig.NatsUserJwt,
				request.HostServicesConfig.NatsUserSeed,
			))

		url = request.HostServicesConfig.NatsUrl
	} else if w.config.HostServicesConfig != nil {
		// FIXME-- check to ensure NATS user JWT and seed are present
		natsOpts = append(natsOpts,
			nats.UserJWTAndSeed(w.config.HostServicesConfig.NatsUserJwt,
				w.config.HostServicesConfig.NatsUserSeed,
			))

		if w.config.HostServicesConfig.NatsUrl != "" {
			url = w.config.HostServicesConfig.NatsUrl
		} else {
			url = w.nc.Servers()[0]
		}
	} else {
		if w.nc.Opts.UserJWT != nil {
			natsOpts = append(natsOpts,
				nats.UserJWT(w.nc.Opts.UserJWT, w.nc.Opts.SignatureCB),
			)
		}

		url = w.nc.Opts.Url
	}

	w.log.Debug("Attempting to establish host services connection for workload",
		slog.String("workload_name", *request.WorkloadName),
		slog.String("url", url),
	)

	nc, err := nats.Connect(url, natsOpts...)
	if err != nil {
		w.log.Warn("Failed to establish host services connection for workload",
			slog.String("workload_name", *request.WorkloadName),
			slog.String("url", url),
			slog.String("error", err.Error()),
		)

		return nil, err
	}

	w.log.Info("Established host services connection for workload",
		slog.String("workload_name", *request.WorkloadName),
		slog.String("url", nc.ConnectedUrl()),
	)

	return nc, nil
}

// Picks a pending agent from the pool that will receive the next deployment
func (w *WorkloadManager) SelectRandomAgent() (*agentapi.AgentClient, error) {
	w.poolMutex.Lock()
	defer w.poolMutex.Unlock()

	// there might be a slightly faster version of this, but this effectively
	// gives us a random pick among the map elements
	for _, agentClient := range w.poolAgents {
		if agentClient.IsSelected() {
			continue
		}

		agentClient.MarkSelected()
		return agentClient, nil
	}

	return nil, errors.New("no available agent client in pool")
}

func (w *WorkloadManager) TotalRunningWorkloadBytes() uint64 {
	total := uint64(0)
	for _, c := range w.liveAgents {
		total += c.WorkloadBytes()
	}

	return total
}

func byteConvert(b float64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%f B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}
