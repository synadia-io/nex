package nexnode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path"
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
	"github.com/synadia-io/nex/internal/models"
	"github.com/synadia-io/nex/internal/node/observability"
)

const (
	systemNamespace              = "system"
	defaultInternalNatsStoreDir  = "pnats"
	heartbeatInterval            = 30 * time.Second
	publicNATSServerStartTimeout = 50 * time.Millisecond
	runloopSleepInterval         = 100 * time.Millisecond
	runloopTickInterval          = 2500 * time.Millisecond
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

	keypair   nkeys.KeyPair
	publicKey string

	natspub *server.Server
	nc      *nats.Conn

	natsint *server.Server
	ncint   *nats.Conn

	startedAt time.Time
	telemetry *observability.Telemetry
}

func NewNode(
	keypair nkeys.KeyPair,
	opts *models.Options,
	nodeOpts *models.NodeOptions,
	ctx context.Context,
	cancelF context.CancelFunc,
	log *slog.Logger) (*Node, error) {
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

func (n *Node) Stop() {
	n.log.Debug("stopping node")
	n.shutdown()
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
	n.pidFilepath = filepath.Join(os.TempDir(), "nex.pid")

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
			n.log.Info("Loaded node configuration", slog.String("config_path", n.nodeOpts.ConfigFilepath))
		}

		n.telemetry, _err = observability.NewTelemetry(n.ctx, n.log, n.config, n.publicKey)
		if _err != nil {
			n.log.Error("Failed to initialize telemetry", slog.Any("err", _err))
			err = errors.Join(err, _err)
		} else {
			n.log.Info("Telemetry status", slog.Bool("metrics", n.config.OtelMetrics), slog.Bool("traces", n.config.OtelTraces))
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
			n.log.Info("Established node NATS connection", slog.String("servers", n.opts.Servers))
		}

		// start internal NATS server
		_err = n.startInternalNATS()
		if _err != nil {
			n.log.Error("Failed to start internal NATS server", slog.Any("err", _err))
			err = errors.Join(err, fmt.Errorf("failed to start internal NATS server: %s", _err))
		} else {
			n.log.Info("Internal NATS server started", slog.String("client_url", n.natsint.ClientURL()))
		}

		n.manager, _err = NewWorkloadManager(n.ctx, n.cancelF, n.keypair, n.publicKey, n.nc, n.ncint, n.config, n.log, n.telemetry)
		if _err != nil {
			n.log.Error("Failed to initialize machine manager", slog.Any("err", _err))
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

func (n *Node) startInternalNATS() error {
	var err error

	n.natsint, err = server.NewServer(&server.Options{
		Host:      "0.0.0.0",
		Port:      -1,
		JetStream: true,
		NoLog:     true,
		StoreDir:  path.Join(os.TempDir(), defaultInternalNatsStoreDir),
	})
	if err != nil {
		return err
	}
	n.natsint.Start()

	clientUrl, err := url.Parse(n.natsint.ClientURL())
	if err != nil {
		return fmt.Errorf("failed to parse internal NATS client URL: %s", err)
	}

	p, err := strconv.Atoi(clientUrl.Port())
	if err != nil {
		return fmt.Errorf("failed to parse internal NATS client URL: %s", err)
	}
	n.config.InternalNodePort = &p

	n.ncint, err = nats.Connect("", nats.InProcessServer(n.natsint))
	if err != nil {
		n.log.Error("Failed to connect to internal nats", slog.Any("err", err), slog.Any("internal_url", clientUrl), slog.Bool("with_jetstream", n.natsint.JetStreamEnabled()))
		return fmt.Errorf("failed to connect to internal nats: %s", err)
	}

	rtt, err := n.ncint.RTT()
	if err != nil {
		n.log.Warn("Failed get internal nats RTT", slog.Any("err", err), slog.Any("internal_url", clientUrl))
	} else {
		n.log.Debug("Internal NATS RTT", slog.String("rtt", rtt.String()), slog.Bool("with_jetstream", n.natsint.JetStreamEnabled()))
	}

	jsCtx, err := n.ncint.JetStream()
	if err != nil {
		return fmt.Errorf("failed to establish jetstream connection to internal nats: %s", err)
	}

	_, err = jsCtx.CreateObjectStore(&nats.ObjectStoreConfig{
		Bucket:      WorkloadCacheBucketName,
		Description: "Object store cache for nex-node workloads",
		Storage:     nats.MemoryStorage,
	})
	if err != nil {
		return fmt.Errorf("failed to create internal object store: %s", err)
	}

	return nil
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

func (n *Node) publishNodeLameDuckEntered() error {
	nodeLameDuck := controlapi.LameDuckEnteredEvent{
		Version: VERSION,
		Id:      n.publicKey,
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
		NodeId:          n.publicKey,
		Version:         Version(),
		Uptime:          myUptime(now.Sub(n.startedAt)),
		RunningMachines: len(machines),
		Tags:            n.config.Tags,
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
		Id:      n.publicKey,
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(n.publicKey)
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(n.startedAt)
	cloudevent.SetType(controlapi.NodeStartedEventType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(nodeStart)

	n.log.Info("Publishing node started event")
	return PublishCloudEvent(n.nc, "system", cloudevent, n.log)
}

func (n *Node) publishNodeStopped() error {
	evt := controlapi.NodeStoppedEvent{
		Id:       n.publicKey,
		Graceful: true,
	}

	cloudevent := cloudevents.NewEvent()
	cloudevent.SetSource(n.publicKey)
	cloudevent.SetID(uuid.NewString())
	cloudevent.SetTime(time.Now().UTC())
	cloudevent.SetType(controlapi.NodeStoppedEventType)
	cloudevent.SetDataContentType(cloudevents.ApplicationJSON)
	_ = cloudevent.SetData(evt)

	n.log.Info("Publishing node stopped event")
	return PublishCloudEvent(n.nc, "system", cloudevent, n.log)
}

func (n *Node) validateConfig() error {
	if n.config == nil {
		err := n.loadNodeConfig()
		if err != nil {
			return err
		}
	}

	return CheckPrerequisites(n.config, true, n.log)
}

func (n *Node) shutdown() {
	if atomic.AddUint32(&n.closing, 1) == 1 {
		n.log.Debug("shutting down")
		_ = n.api.Drain()
		_ = n.manager.Stop()

		if !n.startedAt.IsZero() {
			_ = n.publishNodeStopped()
		}

		_ = n.ncint.Drain()
		for !n.ncint.IsClosed() {
			time.Sleep(time.Millisecond * 25)
		}

		_ = n.nc.Drain()
		for !n.nc.IsClosed() {
			time.Sleep(time.Millisecond * 25)
		}

		n.natsint.Shutdown()
		n.natsint.WaitForShutdown()
		_ = os.Remove(path.Join(os.TempDir(), defaultInternalNatsStoreDir))

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
