package nexnode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/google/uuid"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"github.com/synadia-io/nex/internal/models"
	"github.com/synadia-io/nex/internal/node/observability"
	"github.com/synadia-io/nex/internal/node/preflight"
)

const (
	agentPoolRetryMax            = 100
	autostartAgentRetryMax       = 10
	heartbeatInterval            = 30 * time.Second
	publicNATSServerStartTimeout = 50 * time.Millisecond
	runloopSleepInterval         = 100 * time.Millisecond
	runloopTickInterval          = 2500 * time.Millisecond
	systemNamespace              = "system"
)

// Nex node process
type Node struct {
	api     *ApiListener
	manager *WorkloadManager

	cancelF  context.CancelFunc
	closing  uint32
	lameduck uint32
	ctx      context.Context
	sigs     chan os.Signal

	log *slog.Logger

	config      *models.NodeConfiguration
	opts        *models.Options
	nodeOpts    *models.NodeOptions
	pidFilepath string

	initOnce sync.Once

	keypair       nkeys.KeyPair
	issuerKeypair nkeys.KeyPair
	publicKey     string
	nexus         string

	natspub *server.Server
	nc      *nats.Conn

	startedAt time.Time
	telemetry *observability.Telemetry

	capabilities controlapi.NodeCapabilities
}

func NewNode(
	keypair nkeys.KeyPair,
	opts *models.Options,
	nodeOpts *models.NodeOptions,
	ctx context.Context,
	cancelF context.CancelFunc,
	log *slog.Logger,
) (*Node, error) {
	node := &Node{
		ctx:      ctx,
		cancelF:  cancelF,
		log:      log,
		nodeOpts: nodeOpts,
		opts:     opts,
	}

	err := node.validateConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create node: %s", err.Error())
	}

	err = node.createPid()
	if err != nil {
		return nil, fmt.Errorf("failed to create node: %s", err.Error())
	}

	node.keypair = keypair
	node.publicKey, err = node.keypair.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to extract public key: %s", err.Error())
	}

	// create issuer for signing ad hoc requests
	node.issuerKeypair, _ = nkeys.CreateAccount()

	node.nexus = nodeOpts.NexusName
	node.capabilities = *models.GetNodeCapabilities(node.config.Tags)
	return node, nil
}

func (n *Node) PublicKey() (*string, error) {
	pubkey, err := n.keypair.PublicKey()
	if err != nil {
		return nil, err
	}

	return &pubkey, nil
}

func (n *Node) Start() {
	n.log.Debug("Starting node", slog.String("public_key", n.publicKey))

	err := n.init()
	if err != nil {
		n.shutdown()
		n.cancelF()
		return
	}

	n.startedAt = time.Now()
	_ = n.publishNodeStarted()

	if n.config.AutostartConfiguration != nil {
		go n.handleAutostarts()
	}

	timer := time.NewTicker(runloopTickInterval)
	defer timer.Stop()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for !n.shuttingDown() {
		select {
		case <-timer.C:
			// TODO: check NATS subscription statuses, machine manager, telemetry etc.
		case <-heartbeat.C:
			_ = n.publishHeartbeat()
		case sig := <-n.sigs:
			n.log.Debug("received signal", slog.Any("signal", sig))
			n.shutdown()
		case <-n.ctx.Done():
			n.shutdown()
		default:
			time.Sleep(runloopSleepInterval)
		}
	}

	n.log.Info("exiting node")
	n.cancelF()
}

func (n *Node) EnterLameDuck() error {
	if atomic.AddUint32(&n.lameduck, 1) == 1 {
		n.config.Tags[controlapi.TagLameDuck] = "true"
		err := n.manager.procMan.EnterLameDuck()
		if err != nil {
			return err
		}

		_ = n.publishNodeLameDuckEntered()
	}

	return nil
}

func (n *Node) IsLameDuck() bool {
	return n.lameduck > 0
}

func (n *Node) createPid() error {
	if n.config.PidFilepath != nil {
		n.pidFilepath = *n.config.PidFilepath
	} else {
		n.pidFilepath = filepath.Join(os.TempDir(), "nex.pid")
	}

	var err error
	if _, err = os.Stat(n.pidFilepath); err == nil {
		raw, err := os.ReadFile(n.pidFilepath)
		if err != nil {
			return err
		}

		pid, err := strconv.Atoi(string(raw))
		if err != nil {
			return err
		}

		process, err := os.FindProcess(int(pid))
		if err != nil && !strings.EqualFold(runtime.GOOS, "windows") {
			n.log.Warn("failed to lookup running process by pid", slog.Int("pid", pid), slog.String("err", err.Error()))
			return err
		}

		keepExistingPid := strings.EqualFold(os.Getenv("NEX_ENVIRONMENT"), "spec") // HACK!!! there must be a better way 💀
		if process != nil && !keepExistingPid {
			err = process.Signal(syscall.Signal(0))
			if err == nil {
				return fmt.Errorf("node process already running; pid: %d", pid)
			}
		}
	}

	f, err := os.Create(n.pidFilepath)
	if err != nil {
		return err
	}

	_, err = f.Write([]byte(fmt.Sprintf("%d", os.Getpid())))
	if err != nil {
		_ = os.Remove(n.pidFilepath)
		return err
	}

	n.log.Debug(fmt.Sprintf("Wrote pidfile to %s", n.pidFilepath), slog.Int("pid", os.Getpid()))
	return nil
}

func (n *Node) init() error {
	var err error
	var _err error

	n.initOnce.Do(func() {
		_err = n.loadNodeConfig()
		if _err != nil {
			n.log.Error("Failed to load node configuration file", slog.Any("err", _err), slog.String("config_path", n.nodeOpts.ConfigFilepath))
			err = errors.Join(err, _err)
		} else {
			n.log.Info("Loaded node configuration",
				slog.String("config_path", n.nodeOpts.ConfigFilepath),
			)
		}

		n.telemetry, _err = observability.NewTelemetry(n.ctx, n.log, n.config, n.publicKey)
		if _err != nil {
			n.log.Error("Failed to initialize telemetry", slog.Any("err", _err))
			err = errors.Join(err, _err)
		} else {
			n.log.Debug("Telemetry status", slog.Bool("metrics", n.config.OtelMetrics), slog.Bool("traces", n.config.OtelTraces))
		}

		// start public NATS server
		_err = n.startPublicNATS()
		if _err != nil {
			n.log.Error("Failed to start public NATS server", slog.Any("err", _err))
			err = errors.Join(err, fmt.Errorf("failed to start public NATS server: %s", _err))
		} else if n.natspub != nil {
			n.log.Info("Public NATS server started", slog.String("client_url", n.natspub.ClientURL()))
		}

		// setup NATS connection
		n.nc, _err = models.GenerateConnectionFromOpts(n.opts, n.log)
		if _err != nil {
			n.log.Error("Failed to connect to NATS server", slog.Any("err", _err))
			err = errors.Join(err, fmt.Errorf("failed to connect to NATS server: %s", _err))
		} else {
			n.log.Debug("Established node NATS connection", slog.String("servers", n.opts.Servers))
			n.setConnectionCallbackHandler(n.nc)
		}

		n.manager, _err = NewWorkloadManager(
			n.ctx,
			n.cancelF,
			n.keypair,
			n.publicKey,
			n.nc,
			n.config,
			n.log,
			n.telemetry,
		)
		if _err != nil {
			n.log.Error("Failed to initialize workload manager", slog.Any("err", _err))
			err = errors.Join(err, _err)
		}

		if err == nil {
			go n.manager.Start()

			// init API listener
			n.api = NewApiListener(n.log, n.manager, n)
			_err = n.api.Start()
			if _err != nil {
				n.log.Error("Failed to start API listener", slog.Any("err", _err))
				err = errors.Join(err, _err)
			}
		}

		n.installSignalHandlers()
	})

	return err
}

func (n *Node) startPublicNATS() error {
	if n.config.PublicNATSServer == nil {
		// no-op
		return nil
	}

	var err error
	n.natspub, err = server.NewServer(n.config.PublicNATSServer)
	if err != nil {
		return err
	}

	n.log.Debug("Starting public NATS server")
	n.natspub.Start()

	ports := n.natspub.PortsInfo(publicNATSServerStartTimeout)
	if ports == nil {
		return fmt.Errorf("failed to start public NATS server")
	}

	return nil
}

func (n *Node) handleAutostarts() {
	successCount := 0
	for _, autostart := range n.config.AutostartConfiguration.Workloads {
		var agentClient *agentapi.AgentClient
		var err error

		retry := 0
		for agentClient == nil {
			agentClient, err = n.manager.SelectRandomAgent()
			if err != nil {
				n.log.Warn("Failed to resolve agent for autostart", slog.String("error", err.Error()))
				time.Sleep(50 * time.Millisecond)

				retry += 1
				if retry > autostartAgentRetryMax {
					n.log.Error("Exceeded warm agent retrieval retry count during attempted autostart; terminating node",
						slog.Int("allowed_retries", autostartAgentRetryMax),
					)

					n.shutdown()
					return
				}

				time.Sleep(time.Millisecond * 50)
			}
		}

		// functions cannot be essential
		essential := autostart.Essential && autostart.WorkloadType == controlapi.NexWorkloadNative

		js, err := n.nc.JetStream()
		if err != nil {
			n.log.Error("failed to resolve jetstream",
				slog.String("error", err.Error()),
			)
			continue
		}

		workloadURL, err := url.Parse(autostart.Location)
		if err != nil {
			n.log.Error("failed to parse autostart workload location",
				slog.String("error", err.Error()),
			)
			continue
		}

		bucket, err := js.ObjectStore(workloadURL.Hostname())
		if err != nil {
			n.log.Error("failed to resolve autostart workload object store",
				slog.String("error", err.Error()),
			)
			continue
		}

		artifact := workloadURL.Path[1:len(workloadURL.Path)]
		info, err := bucket.GetInfo(artifact)
		if err != nil {
			n.log.Error("failed to resolve autostart workload artifact",
				slog.String("artifact", artifact),
				slog.String("error", err.Error()),
			)
			continue
		}

		n.log.Debug("resolved autostart workload artifact",
			slog.String("artifact", artifact),
			slog.String("location", workloadURL.String()),
		)

		request, err := controlapi.NewDeployRequest(
			controlapi.Argv(autostart.Argv),
			controlapi.Location(autostart.Location),
			controlapi.Environment(autostart.Environment),
			controlapi.Essential(essential),
			controlapi.Hash(controlapi.SanitizeNATSDigest(info.Digest)),
			controlapi.Issuer(n.issuerKeypair),
			controlapi.SenderXKey(n.api.xk),
			controlapi.TargetNode(n.publicKey),
			controlapi.TargetPublicXKey(n.api.PublicXKey()),
			controlapi.TriggerSubjects(autostart.TriggerSubjects),
			controlapi.WorkloadDescription(*autostart.Description),
			controlapi.WorkloadName(autostart.Name),
			controlapi.WorkloadType(autostart.WorkloadType),
		)
		if err != nil {
			n.log.Error("Failed to create deployment request for autostart workload",
				slog.Any("error", err),
			)
			agentClient.MarkUnselected()
			continue
		}

		if autostart.JsDomain != nil {
			request.JsDomain = autostart.JsDomain
		}

		_, err = request.Validate()
		if err != nil {
			n.log.Error("Failed to validate autostart workload deploy request",
				slog.Any("error", err),
			)
			agentClient.MarkUnselected()
			continue
		}

		// TODO: add potential backoff and retry to cacheworkload
		numBytes, workloadHash, err := n.api.mgr.CacheWorkload(agentClient.ID(), request)
		if err != nil {
			n.api.log.Error("Failed to cache auto-start workload bytes",
				slog.Any("err", err),
				slog.String("name", autostart.Name),
				slog.String("namespace", autostart.Namespace),
				slog.String("url", autostart.Location),
			)
			agentClient.MarkUnselected()
			continue
		}

		var hash string
		if workloadHash != nil {
			hash = *workloadHash // HACK!!! for agent-local workloads, this should be read from the release manifest
		}

		agentWorkloadInfo := agentWorkloadInfoFromControlDeployRequest(request, autostart.Namespace, numBytes, hash)

		agentWorkloadInfo.TotalBytes = int64(numBytes)
		agentWorkloadInfo.Hash = hash

		agentWorkloadInfo.Environment = autostart.Environment // HACK!!! we need to fix autostart config to allow encrypted environment...

		err = n.manager.DeployWorkload(agentClient, agentWorkloadInfo)
		if err != nil {
			n.log.Error("Failed to deploy autostart workload",
				slog.Any("error", err),
				slog.String("name", autostart.Name),
				slog.String("namespace", autostart.Namespace),
			)
			agentClient.MarkUnselected()
			continue
		}

		n.log.Info("Autostart workload started",
			slog.String("name", autostart.Name),
			slog.String("namespace", autostart.Namespace),
			slog.String("workload_id", agentClient.ID()),
		)

		successCount += 1
	}

	if successCount < len(n.config.AutostartConfiguration.Workloads) {
		n.log.Error("Failed to initialize autostart workloads",
			slog.Int("expected", len(n.config.AutostartConfiguration.Workloads)),
			slog.Int("actual", successCount),
		)
	}
}

func (n *Node) loadNodeConfig() error {
	if n.config == nil {
		var err error

		n.config, err = LoadNodeConfiguration(n.nodeOpts.ConfigFilepath)
		if err != nil {
			return err
		}

		// HACK-- copying these here... everything should ultimately be configurable via node JSON config...
		n.config.OtelMetrics = n.nodeOpts.OtelMetrics
		n.config.OtelMetricsExporter = n.nodeOpts.OtelMetricsExporter
		n.config.OtelMetricsPort = n.nodeOpts.OtelMetricsPort
		n.config.OtelTraces = n.nodeOpts.OtelTraces
		n.config.OtelTracesExporter = n.nodeOpts.OtelTracesExporter
	}

	return nil
}

func (n *Node) ncClosedHandler(conn *nats.Conn) {
	attrs := make([]any, 0)
	attrs = append(attrs, slog.String("url", conn.Servers()[0]))

	if conn.Opts.Name != "" {
		attrs = append(attrs, slog.String("name", conn.Opts.Name))
	}

	n.log.Debug("NATS connection closed", attrs...)
}

func (n *Node) ncDisconnectErrorHandler(conn *nats.Conn, err error) {
	attrs := make([]any, 0)
	attrs = append(attrs, slog.String("url", conn.Servers()[0]))

	if conn.Opts.Name != "" {
		attrs = append(attrs, slog.String("name", conn.Opts.Name))
	}

	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}

	n.log.Debug("NATS connection disconnected", attrs...)
}

func (n *Node) ncErrorHandler(conn *nats.Conn, _ *nats.Subscription, err error) {
	attrs := make([]any, 0)
	attrs = append(attrs, slog.String("url", conn.Servers()[0]))

	if conn.Opts.Name != "" {
		attrs = append(attrs, slog.String("name", conn.Opts.Name))
	}

	attrs = append(attrs, slog.Any("error", err))

	n.log.Error("NATS error", attrs...)
}

func (n *Node) ncReconnectedHandler(conn *nats.Conn) {
	attrs := make([]any, 0)
	attrs = append(attrs, slog.String("url", conn.Servers()[0]))

	if conn.Opts.Name != "" {
		attrs = append(attrs, slog.String("name", conn.Opts.Name))
	}

	n.log.Debug("NATS connection re-established", attrs...)
}

func (n *Node) publishNodeLameDuckEntered() error {
	nodeLameDuck := controlapi.LameDuckEnteredEvent{
		Version: VERSION,
		ID:      n.publicKey,
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(n.publicKey)
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(n.startedAt)
	cloudevent.SetType(controlapi.LameDuckEnteredEventType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(nodeLameDuck)

	n.log.Info("Publishing node lame duck entered event")
	return PublishCloudEvent(n.nc, "system", cloudevent, n.log)
}

func (n *Node) publishHeartbeat() error {
	machines, err := n.manager.RunningWorkloads()
	if err != nil {
		n.log.Error("Failed to query running machines during heartbeat", slog.Any("error", err))
		return nil
	}

	now := time.Now().UTC()

	evt := controlapi.HeartbeatEvent{
		AllowDuplicateWorkloads: n.config.AllowDuplicateWorkloads,
		Nexus:                   n.nexus,
		NodeID:                  n.publicKey,
		RunningMachines:         len(machines),
		Tags:                    n.config.Tags,
		Uptime:                  myUptime(now.Sub(n.startedAt)),
		Version:                 Version(),
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(n.publicKey)
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(now)
	cloudevent.SetType(controlapi.HeartbeatEventType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(evt)

	return PublishCloudEvent(n.nc, systemNamespace, cloudevent, n.log)
}

func (n *Node) publishNodeStarted() error {
	nodeStart := controlapi.NodeStartedEvent{
		Version: VERSION,
		ID:      n.publicKey,
		Tags:    n.config.Tags,
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(n.publicKey)
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(n.startedAt)
	cloudevent.SetType(controlapi.NodeStartedEventType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(nodeStart)

	n.log.Debug("Publishing node started event")
	return PublishCloudEvent(n.nc, "system", cloudevent, n.log)
}

func (n *Node) publishNodeStopped() error {
	evt := controlapi.NodeStoppedEvent{
		ID:       n.publicKey,
		Graceful: true,
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(n.publicKey)
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType(controlapi.NodeStoppedEventType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(evt)

	n.log.Debug("Publishing node stopped event")
	return PublishCloudEvent(n.nc, "system", cloudevent, n.log)
}

func (n *Node) setConnectionCallbackHandler(nc *nats.Conn) {
	nc.SetClosedHandler(n.ncClosedHandler)
	nc.SetDisconnectErrHandler(n.ncDisconnectErrorHandler)
	nc.SetErrorHandler(n.ncErrorHandler)
	nc.SetReconnectHandler(n.ncReconnectedHandler)

	n.log.Debug("Set NATS connection callback handlers", slog.String("servers", n.opts.Servers))
}

func (n *Node) validateConfig() error {
	if n.config == nil {
		err := n.loadNodeConfig()
		if err != nil {
			return err
		}
	}

	return preflight.Validate(n.config, n.log)
}

func (n *Node) shutdown() {
	if atomic.AddUint32(&n.closing, 1) == 1 {
		n.log.Debug("shutting down")
		if n.api != nil {
			_ = n.api.Drain()
			_ = n.nc.Flush()
		}

		if n.manager != nil {
			_ = n.manager.Stop()
		}

		if !n.startedAt.IsZero() {
			_ = n.publishNodeStopped()
		}

		if n.nc != nil {
			_ = n.nc.Drain()
			for !n.nc.IsClosed() {
				time.Sleep(time.Millisecond * 25)
			}
		}

		if n.natspub != nil {
			n.natspub.Shutdown()
			n.natspub.WaitForShutdown()
		}

		_ = n.telemetry.Shutdown()

		_ = os.Remove(n.pidFilepath)

		signal.Stop(n.sigs)
		close(n.sigs)
	}
}

func (n *Node) shuttingDown() bool {
	return (atomic.LoadUint32(&n.closing) > 0)
}

func agentWorkloadInfoFromControlDeployRequest(request *controlapi.DeployRequest, namespace string, numBytes uint64, hash string) *agentapi.AgentWorkloadInfo {
	return &agentapi.AgentWorkloadInfo{
		Argv:                 request.Argv,
		DecodedClaims:        request.DecodedClaims,
		Description:          request.Description,
		EncryptedEnvironment: request.Environment,
		Environment:          request.WorkloadEnvironment, // HACK!!! we need to fix autostart config to allow encrypted environment...
		Essential:            request.Essential,
		Hash:                 hash,
		HostServicesConfig:   request.HostServicesConfig,
		ID:                   request.ID,
		Location:             request.Location,
		Namespace:            &namespace,
		RetriedAt:            request.RetriedAt,
		RetryCount:           request.RetryCount,
		SenderPublicKey:      request.SenderPublicKey,
		TotalBytes:           int64(numBytes),
		TriggerSubjects:      request.TriggerSubjects,
		WorkloadJWT:          request.WorkloadJWT,
		WorkloadName:         request.WorkloadName,
		WorkloadType:         request.WorkloadType,
	}
}
