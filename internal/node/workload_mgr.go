package nexnode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
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
	activeAgents map[string]*agentapi.AgentClient

	// Agent clients in this slice are attached to processes that have not yet received a deployment AND have
	// successfully performed a handshake. Handshake failures are immediately removed
	pendingAgents map[string]*agentapi.AgentClient

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

		pendingAgents: make(map[string]*agentapi.AgentClient),
		activeAgents:  make(map[string]*agentapi.AgentClient),

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
		w.log.Info("Internal NATS server started", slog.String("client_url", w.natsint.ClientURL()))
	}

	w.hostServices = NewHostServices(w.ncint, config.HostServicesConfiguration, w.log, w.t.Tracer)
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
	w.log.Info("Workload manager starting")

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

	workload, err := store.GetBytes(key)
	if err != nil {
		m.log.Error("Failed to download bytes from source object store", slog.Any("err", err), slog.String("key", key))
		return 0, nil, err
	}

	err = m.natsint.StoreFileForID(workloadID, workload)
	if err != nil {
		m.log.Error("Failed to store bytes from source object store in cache", slog.Any("err", err), slog.String("key", key))
	}

	workloadHash := sha256.New()
	workloadHash.Write(workload)
	workloadHashString := hex.EncodeToString(workloadHash.Sum(nil))

	m.log.Info("Successfully stored workload in internal object store",
		slog.String("name", request.DecodedClaims.Subject),
		slog.Int("bytes", len(workload)))

	return uint64(len(workload)), &workloadHashString, nil
}

// Deploy a workload as specified by the given deploy request to an available
// agent in the configured pool
func (w *WorkloadManager) DeployWorkload(agentClient *agentapi.AgentClient, request *agentapi.DeployRequest) error {
	w.poolMutex.Lock()
	defer w.poolMutex.Unlock()

	workloadID := agentClient.ID()
	err := w.procMan.PrepareWorkload(workloadID, request)
	if err != nil {
		return fmt.Errorf("failed to prepare agent process for workload deployment: %s", err)
	}

	status := w.ncint.Status()

	w.log.Debug("Workload manager deploying workload",
		slog.String("workload_id", workloadID),
		slog.String("conn_status", status.String()))

	deployResponse, err := agentClient.DeployWorkload(request)
	if err != nil {
		return fmt.Errorf("failed to submit request for workload deployment: %s", err)
	}

	if deployResponse.Accepted {
		// move the client from active to pending
		w.activeAgents[workloadID] = agentClient
		delete(w.pendingAgents, workloadID)

		ncHostServices, err := w.createHostServicesConnection(request)
		if err != nil {
			w.log.Error("Failed to establish host services connection for workload",
				slog.Any("error", err),
			)
			return err
		}

		w.hostServices.server.SetHostServicesConnection(workloadID, ncHostServices)

		if request.SupportsTriggerSubjects() {
			ncTrigger := ncHostServices
			if request.TriggerConnection != nil {
				ncTrigger, err = createJwtConnection(request.TriggerConnection)
				if err != nil {
					w.log.Error("Failed to create trigger connection",
						slog.Any("error", err),
						slog.String("url", request.TriggerConnection.NatsUrl),
					)
					_ = w.StopWorkload(workloadID, true)
					return err
				}
				w.hostServices.server.AddHostServicesConnection(workloadID, ncTrigger)
			}
			for _, tsub := range request.TriggerSubjects {
				sub, err := ncTrigger.Subscribe(tsub, w.generateTriggerHandler(workloadID, tsub, request))
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
					slog.String("nats_url", ncTrigger.ConnectedAddr()),
					slog.String("trigger_subject", tsub),
					slog.String("workload_type", string(request.WorkloadType)),
				)

				w.subz[workloadID] = append(w.subz[workloadID], sub)
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
			if p.DeployRequest.WorkloadType == controlapi.NexWorkloadV8 || p.DeployRequest.WorkloadType == controlapi.NexWorkloadWasm {
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
				WorkloadType: p.DeployRequest.WorkloadType,
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

		for id := range w.pendingAgents {
			_ = w.pendingAgents[id].Stop()
		}

		for id := range w.activeAgents {
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
		_ = os.Remove(path.Join(os.TempDir(), defaultInternalNatsStoreDir))
	}

	return nil
}

// Stop a workload, optionally attempting a graceful undeploy prior to termination
func (w *WorkloadManager) StopWorkload(id string, undeploy bool) error {
	defer func() {
		delete(w.activeAgents, id)
		delete(w.pendingAgents, id)
		delete(w.stopMutex, id)
		w.hostServices.server.RemoveHostServicesConnection(id)

		_ = w.publishWorkloadStopped(id)
	}()

	deployRequest, err := w.procMan.Lookup(id)
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
		agentClient := w.activeAgents[id]
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

	return nil
}

// Called by the agent process manager when an agent has been warmed and is ready
// to receive workload deployment instructions
func (w *WorkloadManager) OnProcessStarted(id string) {
	w.log.Debug("Process started", slog.String("workload_id", id))
	w.poolMutex.Lock()
	defer w.poolMutex.Unlock()

	clientConn, err := w.natsint.ConnectionWithID(id)
	if err != nil {
		w.log.Error("Failed to start agent client", slog.Any("err", err))
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

func (w *WorkloadManager) agentContactLost(workloadID string) {
	w.log.Warn("Lost contact with agent", slog.String("workload_id", workloadID))
	_ = w.StopWorkload(workloadID, false)
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
			_ = w.publishFunctionExecFailed(workloadID, *request.WorkloadName, *request.Namespace, tsub, err)
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
	var err error
	w.natsint, err = internalnats.NewInternalNatsServer(w.log)
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
	} else if w.config.HostServicesConfiguration != nil {
		// FIXME-- check to ensure NATS user JWT and seed are present
		natsOpts = append(natsOpts,
			nats.UserJWTAndSeed(w.config.HostServicesConfiguration.NatsUserJwt,
				w.config.HostServicesConfiguration.NatsUserSeed,
			))

		if w.config.HostServicesConfiguration.NatsUrl != "" {
			url = w.config.HostServicesConfiguration.NatsUrl
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

func createJwtConnection(info *controlapi.NatsJwtConnectionInfo) (*nats.Conn, error) {
	natsOpts := []nats.Option{
		nats.Name("workload-trigger-subscription"),
	}
	natsOpts = append(natsOpts,
		nats.UserJWTAndSeed(info.NatsUserJwt,
			info.NatsUserSeed,
		))

	return nats.Connect(info.NatsUrl, natsOpts...)
}
