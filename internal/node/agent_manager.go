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
	"strings"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"github.com/synadia-io/nex/internal/models"
	internalnats "github.com/synadia-io/nex/internal/node/internal-nats"
	"github.com/synadia-io/nex/internal/node/observability"
)

// An agent manager is incrementally... a WorkloadManager manager
type AgentManager struct {
	cancel    context.CancelFunc
	closing   uint32
	config    *models.NodeConfiguration
	ctx       context.Context
	kp        nkeys.KeyPair
	log       *slog.Logger
	publicKey string
	t         *observability.Telemetry

	managers map[string]*WorkloadManager

	nc      *nats.Conn
	natsint *internalnats.InternalNatsServer
	ncint   *nats.Conn

	hostServices *HostServices
}

func NewAgentManager(
	ctx context.Context,
	cancel context.CancelFunc,
	nodeKeypair nkeys.KeyPair,
	publicKey string,
	nc *nats.Conn,
	config *models.NodeConfiguration,
	log *slog.Logger,
	telemetry *observability.Telemetry,
) (*AgentManager, error) {
	a := &AgentManager{
		cancel:    cancel,
		config:    config,
		ctx:       ctx,
		kp:        nodeKeypair,
		log:       log,
		managers:  map[string]*WorkloadManager{},
		nc:        nc,
		publicKey: publicKey,
		t:         telemetry,
	}

	// start internal NATS server
	err := a.startInternalNATS()
	if err != nil {
		a.log.Error("Failed to start internal NATS server", slog.Any("err", err))
		return nil, err
	} else {
		a.log.Debug("Internal NATS server started", slog.String("client_url", a.natsint.ClientURL()))
	}

	a.hostServices = NewHostServices(a.ncint, config.HostServicesConfig, a.log, a.t.Tracer)
	err = a.hostServices.init()
	if err != nil {
		a.log.Warn("Failed to initialize host services", slog.Any("err", err))
		return nil, err
	}

	err = a.initAgents()
	if err != nil {
		a.log.Warn("Failed to initialize agents for configured workload types", slog.Any("err", err))
		return nil, err
	}

	return a, nil
}

func (a *AgentManager) initAgents() error {
	var err error

	for _, workloadType := range a.config.WorkloadTypes {
		// TODO-- lookup agent using new uri-based design
		a.log.Debug("Agent manager initializing support for workload type", slog.String("workload_type", string(workloadType)))

		manager, _err := NewWorkloadManager(
			a.ctx,
			a.cancel,
			a.hostServices,
			a.kp,
			a.publicKey,
			a.nc,
			a.natsint,
			a.ncint,
			a.config,
			a.log,
			a.t,
			workloadType,
		)
		if _err != nil {
			a.log.Error("Failed to initialize workload manager", slog.Any("err", _err))
			err = errors.Join(err, _err)
			continue
		}

		a.managers[string(workloadType)] = manager
	}

	return err
}

func (a *AgentManager) startInternalNATS() error {
	storeDir := filepath.Join(os.TempDir(), defaultInternalNatsStoreDir)
	if a.config.InternalNodeStoreDir != nil {
		storeDir = *a.config.InternalNodeStoreDir
	}

	var err error
	a.natsint, err = internalnats.NewInternalNatsServer(a.log, storeDir, a.config.InternalNodeDebug, a.config.InternalNodeTrace)
	if err != nil {
		return err
	}

	p := a.natsint.Port()
	a.config.InternalNodePort = &p
	a.ncint = a.natsint.Connection()

	return nil
}

func (a *AgentManager) AvailableAgentsCount() int {
	availableAgents := 0

	for id := range a.managers {
		availableAgents += len(a.managers[id].poolAgents) // FIXME-- denormalize this count on the managers like RunningWorkloadCount()
	}

	return availableAgents
}

func (a *AgentManager) CacheWorkload(workloadID string, request *controlapi.DeployRequest) (uint64, *string, error) {
	bucket := request.Location.Host
	key := strings.Trim(request.Location.Path, "/")

	jsLogAttr := []any{slog.String("bucket", bucket), slog.String("key", key)}
	opts := []nats.JSOpt{}
	if request.JsDomain != nil {
		opts = append(opts, nats.Domain(*request.JsDomain))

		jsLogAttr = append(jsLogAttr, slog.String("jsdomain", *request.JsDomain))
	}

	a.log.Info("Attempting object store download", jsLogAttr...)

	js, err := a.nc.JetStream(opts...)
	if err != nil {
		return 0, nil, err
	}

	store, err := js.ObjectStore(bucket)
	if err != nil {
		a.log.Error("Failed to bind to source object store", slog.Any("err", err), slog.String("bucket", bucket))
		return 0, nil, err
	}

	_, err = store.GetInfo(key)
	if err != nil {
		a.log.Error("Failed to locate workload binary in source object store", slog.Any("err", err), slog.String("key", key), slog.String("bucket", bucket))
		return 0, nil, err
	}

	started := time.Now()
	workload, err := store.GetBytes(key)
	if err != nil {
		a.log.Error("Failed to download bytes from source object store", slog.Any("err", err), slog.String("key", key))
		return 0, nil, err
	}
	finished := time.Since(started)
	dlRate := float64(len(workload)) / finished.Seconds()
	a.log.Debug("CacheWorkload object store download completed", slog.String("name", key), slog.String("duration", fmt.Sprintf("%.2f sec", finished.Seconds())), slog.String("rate", fmt.Sprintf("%s/sec", byteConvert(dlRate))))

	err = a.natsint.StoreFileForID(workloadID, workload)
	if err != nil {
		a.log.Error("Failed to store bytes from source object store in cache", slog.Any("err", err), slog.String("key", key))
	}

	workloadHash := sha256.New()
	workloadHash.Write(workload)
	workloadHashString := hex.EncodeToString(workloadHash.Sum(nil))

	a.log.Info("Successfully stored workload in internal object store",
		slog.String("workload_name", *request.WorkloadName),
		slog.String("workload_id", workloadID),
		slog.String("workload_hash", workloadHashString),
		slog.Int("bytes", len(workload)),
	)

	return uint64(len(workload)), &workloadHashString, nil
}

// FIXME-- we have been leaking AgentClient since one of our previous refactors...
// to fix: resolve the appropriate agent client from within AgentManager...
func (a *AgentManager) DeployWorkload(agentClient *agentapi.AgentClient, request *agentapi.AgentWorkloadInfo) error {
	if manager, ok := a.managers[string(request.WorkloadType)]; ok {
		return manager.DeployWorkload(agentClient, request)
	}

	return fmt.Errorf("unsupported workload type: %s", request.WorkloadType)
}

func (a *AgentManager) EnterLameDuck() error {
	var err error

	for id := range a.managers {
		_err := a.managers[id].EnterLameDuck()
		if _err != nil {
			err = errors.Join(_err)
		}
	}

	return err
}

func (a *AgentManager) LookupWorkload(workloadID string) (*agentapi.AgentWorkloadInfo, error) {
	for id := range a.managers {
		if agentClient, ok := a.managers[id].liveAgents[workloadID]; ok {
			return agentClient.WorkloadInfo(), nil
		}
	}

	return nil, fmt.Errorf("workload doesn't exist: %s", workloadID)
}

func (a *AgentManager) RunningWorkloadCount() int32 {
	runningWorkloads := int32(0)

	for id := range a.managers {
		runningWorkloads += a.managers[id].RunningWorkloadCount()
	}

	return runningWorkloads
}

func (a *AgentManager) RunningWorkloads() ([]controlapi.MachineSummary, error) {
	workloads := make([]controlapi.MachineSummary, 0)

	for id := range a.managers {
		wrkloads, err := a.managers[id].RunningWorkloads()
		if err != nil {
			a.log.Error("Failed to list running workloads for workload type: %s", id, slog.Any("error", err))
			return nil, err
		}

		workloads = append(workloads, wrkloads...)
	}

	return workloads, nil
}

func (a *AgentManager) SelectAgent(workloadType controlapi.NexWorkload) (*agentapi.AgentClient, error) {
	if manager, ok := a.managers[string(workloadType)]; ok {
		return manager.SelectAgent(a.isMultiTenant(workloadType))
	}

	return nil, fmt.Errorf("unsupported workload type: %s", workloadType) // unreachable
}

func (a *AgentManager) StopWorkload(workloadID string, undeploy bool) error {
	for id := range a.managers {
		_, err := a.managers[id].LookupWorkload(workloadID)
		if err == nil {
			return a.managers[id].StopWorkload(id, undeploy)
		}
	}

	return fmt.Errorf("failed to stop workload: %s", workloadID)
}

func (a *AgentManager) TotalRunningWorkloadBytes() uint64 {
	total := uint64(0)

	for id := range a.managers {
		total += a.managers[id].TotalRunningWorkloadBytes()
	}

	return total
}

// Start the agent manager, which in turn starts the configured workload managers
func (a *AgentManager) Start() {
	a.log.Debug("Agent manager starting")

	for id := range a.managers {
		go func() {
			err := a.managers[id].Start()
			if err != nil {
				a.log.Error("Agent manager failed to start", slog.Any("error", err))
				a.Stop()
				a.cancel()

				return
			}
		}()
	}
}

// Stop the agent manager, which will in turn stop all managed workload managers and cleanup
func (a *AgentManager) Stop() error {
	if atomic.AddUint32(&a.closing, 1) == 1 {
		for id := range a.managers {
			_ = a.managers[id].Stop()
		}

		_ = a.ncint.Drain()
		for !a.ncint.IsClosed() {
			time.Sleep(time.Millisecond * 25)
		}

		a.natsint.Shutdown()

		a.log.Debug("Agent manager stopped")
	}

	return nil
}

func (a *AgentManager) isMultiTenant(workloadType controlapi.NexWorkload) bool {
	return workloadType != controlapi.NexWorkloadNative
}
