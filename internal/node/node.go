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

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/models"
)

const defaultNatsStoreDir = "pnats"

const runloopSleepInterval = 250 * time.Millisecond
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

	nc *nats.Conn

	natsint *server.Server
	ncint   *nats.Conn

	telemetry *Telemetry
}

func Up(opts *models.Options, nodeOpts *models.NodeOptions, ctx context.Context, cancelF context.CancelFunc, log *slog.Logger) (*Node, error) {
	node := &Node{
		ctx:      ctx,
		cancelF:  cancelF,
		log:      log,
		nodeOpts: nodeOpts,
		opts:     opts,
	}

	err := node.preflight()
	if err != nil {
		return nil, fmt.Errorf("failed to start node: %s", err.Error())
	}

	go node.Start()

	return node, nil
}

func (n *Node) Start() {
	n.log.Debug("starting node")

	err := n.init()
	if err != nil {
		panic(err) // FIXME-- this panics here because this is written like a proper main() entrypoint (it should never actually panic in practice)
	}

	// startAt := time.Now()

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
			close(n.sigs)
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

func (n *Node) init() error {
	var err error

	n.initOnce.Do(func() {
		n.telemetry, err = NewTelemetry()
		if err != nil {
			n.log.Error("Failed to initialize telemetry", slog.Any("err", err))
			err = fmt.Errorf("failed to initialize telemetry: %s", err)
		}
		n.log.Info("Initialized telemetry")

		err = n.loadNodeConfig()

		// setup NATS connection
		n.nc, err = models.GenerateConnectionFromOpts(n.opts)
		if err != nil {
			n.log.Error("Failed to connect to NATS server", slog.Any("err", err))
			err = fmt.Errorf("failed to connect to NATS server: %s", err)
		}
		n.log.Info("Established node NATS connection", slog.String("servers", n.opts.Servers))

		// init internal NATS server
		err = n.initInternalNATS()
		if err != nil {
			n.log.Error("Failed to initialize internal NATS server", slog.Any("err", err))
			err = fmt.Errorf("failed to initialize internal NATS server: %s", err)
		}
		n.log.Info("Internal NATS server started", slog.String("client_url", n.natsint.ClientURL()))

		// init machine manager
		n.manager, err = NewMachineManager(n.ctx, n.nc, n.ncint, n.config, n.log, n.telemetry)
		if err != nil {
			n.log.Error("Failed to initialize machine manager", slog.Any("err", err))
			err = fmt.Errorf("failed to initialize machine manager: %s", err)
		}

		err = n.manager.Start()
		if err != nil {
			n.log.Error("Failed to start machine manager", slog.Any("err", err))
			err = fmt.Errorf("failed to start machine manager: %s", err)
		}

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
			n.log.Error("Failed to load node configuration file", slog.Any("err", err), slog.String("config_path", n.nodeOpts.ConfigFilepath))
			err = fmt.Errorf("failed to load node configuration file: %s", err)
			return err
		}

		n.log.Info("Loaded node configuration from '%s'", slog.String("config_path", n.nodeOpts.ConfigFilepath))
	}

	return nil
}

func (n *Node) preflight() error {
	if n.config == nil {
		err := n.loadNodeConfig()
		if err != nil {
			return err
		}
	}

	return CheckPreRequisites(n.config, false) // FIXME?
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
		n.nc.Close()
		n.ncint.Close()
		n.natsint.Shutdown()
		n.natsint.WaitForShutdown()
	}
}

func (n *Node) shuttingDown() bool {
	return (atomic.LoadUint32(&n.closing) > 0)
}
