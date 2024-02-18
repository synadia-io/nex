package nexnode

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/google/uuid"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	controlapi "github.com/synadia-io/nex/internal/control-api"
	"github.com/synadia-io/nex/internal/models"
	"go.opentelemetry.io/otel/trace/noop"
)

const defaultNatsStoreDir = "pnats"
const defaultPidFilepath = "/var/run/nex.pid"

const runloopSleepInterval = 100 * time.Millisecond
const runloopTickInterval = 2500 * time.Millisecond

// Nex node process
type Node struct {
	api     *ApiListener
	manager *MachineManager

	cancelF context.CancelFunc
	closing uint32
	ctx     context.Context
	sigs    chan os.Signal

	log *slog.Logger

	config   *NodeConfiguration
	opts     *models.Options
	nodeOpts *models.NodeOptions

	initOnce sync.Once

	keypair   nkeys.KeyPair
	publicKey string

	dns *DNS
	nc  *nats.Conn

	natsint *server.Server
	ncint   *nats.Conn

	startedAt time.Time
	telemetry *Telemetry
}

func NewNode(opts *models.Options, nodeOpts *models.NodeOptions, ctx context.Context, cancelF context.CancelFunc, log *slog.Logger) (*Node, error) {
	node := &Node{
		ctx:      ctx,
		cancelF:  cancelF,
		log:      log,
		nodeOpts: nodeOpts,
		opts:     opts,
	}

	err := node.validateConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to start node: %s", err.Error())
	}

	err = node.createPid()
	if err != nil {
		return nil, fmt.Errorf("failed to start node: %s", err.Error())
	}

	err = node.generateKeypair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate keypair for node: %s", err)
	} else {
		node.log.Info("Generated keypair for node", slog.String("public_key", node.publicKey))
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
	n.log.Debug("starting node", slog.String("public_key", n.publicKey))
	n.startedAt = time.Now()

	err := n.init()
	if err != nil {
		panic(err) // FIXME-- this panics here because this is written like a proper main() entrypoint (it should never actually panic in practice)
	}

	_ = n.publishNodeStarted()

	timer := time.NewTicker(runloopTickInterval)
	defer timer.Stop()

	for !n.shuttingDown() {
		select {
		case <-timer.C:
			// TODO: check NATS subscription statuses, machine manager, telemetry etc.
		case sig := <-n.sigs:
			n.log.Debug("received signal: %s", sig)
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

func (n *Node) createPid() error {
	if _, err := os.Stat(defaultPidFilepath); err == nil {
		raw, err := os.ReadFile(defaultPidFilepath)
		if err != nil {
			return err
		}

		pid, err := strconv.Atoi(string(raw))
		if err != nil {
			return err
		}

		process, err := os.FindProcess(int(pid))
		if err != nil {
			return err
		}

		err = process.Signal(syscall.Signal(0))
		if err == nil {
			return fmt.Errorf("node process already running; pid: %d", pid)
		}
	}

	f, err := os.Create(defaultPidFilepath)
	if err != nil {
		return err
	}

	_, err = f.Write([]byte(fmt.Sprintf("%d", os.Getpid())))
	if err != nil {
		_ = os.Remove(defaultPidFilepath)
		return err
	}

	n.log.Debug(fmt.Sprintf("Wrote pidfile to %s", defaultPidFilepath), slog.Int("pid", os.Getpid()))
	return nil
}

func (n *Node) generateKeypair() error {
	var err error

	n.keypair, err = nkeys.CreateServer()
	if err != nil {
		return fmt.Errorf("failed to generate node keypair: %s", err)
	}

	n.publicKey, err = n.keypair.PublicKey()
	if err != nil {
		return fmt.Errorf("failed to encode public key: %s", err)
	}

	return nil
}

func (n *Node) init() error {
	var err error

	n.initOnce.Do(func() {
		err = n.loadNodeConfig()
		if err != nil {
			n.log.Error("Failed to load node configuration file", slog.Any("err", err), slog.String("config_path", n.nodeOpts.ConfigFilepath))
		} else {
			n.log.Info("Loaded node configuration", slog.String("config_path", n.nodeOpts.ConfigFilepath))
		}

		n.telemetry, err = NewTelemetry(n.ctx, n.log, n.config, n.publicKey)
		if err != nil {
			n.log.Error("Failed to initialize telemetry", slog.Any("err", err))
		} else {
			n.log.Info("Initialized telemetry")
		}

		if n.config.OtlpExporterUrl != nil {
			err = InitializeTraceProvider(*n.config.OtlpExporterUrl)
			if err != nil {
				n.log.Error("Failed to initialize OTLP trace exporter", slog.Any("err", err),
					slog.String("url", *n.config.OtlpExporterUrl),
				)
				tracer = noop.NewTracerProvider().Tracer("nex")
			} else {
				n.log.Info("Initialized OTLP exporter",
					slog.String("url", *n.config.OtlpExporterUrl),
				)
			}
		} else {
			tracer = noop.NewTracerProvider().Tracer("nex")
		}

		// setup DNS nameserver
		n.dns, err = NewDNS(n.log)
		if err != nil {
			n.log.Error("Failed to initialize DNS nameserver", slog.Any("err", err))
			err = fmt.Errorf("failed to initialize DNS nameserver: %s", err)
		} else {
			n.log.Info("Initialized DNS nameserver")
		}

		// setup NATS connection
		n.nc, err = models.GenerateConnectionFromOpts(n.opts)
		if err != nil {
			n.log.Error("Failed to connect to NATS server", slog.Any("err", err))
			err = fmt.Errorf("failed to connect to NATS server: %s", err)
		} else {
			n.log.Info("Established node NATS connection", slog.String("servers", n.opts.Servers))
		}

		// init internal NATS server
		err = n.initInternalNATS()
		if err != nil {
			n.log.Error("Failed to initialize internal NATS server", slog.Any("err", err))
			err = fmt.Errorf("failed to initialize internal NATS server: %s", err)
		} else {
			n.log.Info("Internal NATS server started", slog.String("client_url", n.natsint.ClientURL()))
		}

		// init machine manager
		n.manager, err = NewMachineManager(n.ctx, n.cancelF, n.keypair, n.publicKey, n.nc, n.ncint, n.config, n.log, n.dns, n.telemetry)
		if err != nil {
			n.log.Error("Failed to initialize machine manager", slog.Any("err", err))
			err = fmt.Errorf("failed to initialize machine manager: %s", err)
		}

		go n.manager.Start()

		// init API listener
		n.api = NewApiListener(n.log, n.manager, n.config)
		err = n.api.Start()
		if err != nil {
			n.log.Error("Failed to start API listener", slog.Any("err", err))
			err = fmt.Errorf("failed to start node API: %s", err)
		}

		n.installSignalHandlers()
	})

	return err
}

func (n *Node) initInternalNATS() error {
	var err error

	n.natsint, err = server.NewServer(&server.Options{
		Host:      "0.0.0.0",
		Port:      -1,
		JetStream: true,
		NoLog:     true,
		StoreDir:  path.Join(os.TempDir(), defaultNatsStoreDir),
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

	n.ncint, err = nats.Connect(n.natsint.ClientURL())
	if err != nil {
		return fmt.Errorf("failed to connect to internal nats: %s", err)
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
	}

	return nil
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

	return CheckPrerequisites(n.config, true)
}

func (n *Node) installSignalHandlers() {
	n.log.Debug("installing signal handlers")
	// both firecracker and the embedded NATS server register signal handlers... wipe those so ours are the ones being used
	signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2, syscall.SIGHUP)
	n.sigs = make(chan os.Signal, 1)
	signal.Notify(n.sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
}

func (n *Node) shutdown() {
	if atomic.AddUint32(&n.closing, 1) == 1 {
		n.log.Debug("shutting down")
		_ = n.manager.Stop()
		_ = n.publishNodeStopped()

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
		_ = n.telemetry.Shutdown()
		_ = n.dns.Stop()

		_ = os.Remove(defaultPidFilepath)
		close(n.sigs)
	}
}

func (n *Node) shuttingDown() bool {
	return (atomic.LoadUint32(&n.closing) > 0)
}
