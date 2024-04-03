package nexnode

import (
	"context"
	"log/slog"

	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

type SpawningProcessManager struct {
	//closing uint32
	config *NodeConfiguration
	ctx    context.Context
	//stopMutex map[string]*sync.Mutex
	t *Telemetry

	delegate       AgentDelegate
	deployRequests map[string]*agentapi.DeployRequest

	log *slog.Logger
}

func NewSpawningProcessManager(
	log *slog.Logger,
	config *NodeConfiguration,
	telemetry *Telemetry,
	ctx context.Context,
) (*SpawningProcessManager, error) {

	return &SpawningProcessManager{
		config: config,
		t:      telemetry,
		log:    log,
		ctx:    ctx,

		deployRequests: make(map[string]*agentapi.DeployRequest),
	}, nil
}

func (s *SpawningProcessManager) ListProcesses() ([]AgentInfo, error) {
	//pinfos := make([]ProcessInfo, 0)

	panic("Not implemented")
	// TODO
}

func (s *SpawningProcessManager) ListPool() ([]AgentInfo, error) {
	//pinfos := make([]ProcessInfo, 0)

	panic("Not implemented")
	// TODO
}

func (s *SpawningProcessManager) PrepareWorkload(agentID string, deployRequest *agentapi.DeployRequest) error {
	panic("Not implemented")
}

func (s *SpawningProcessManager) Stop() error {
	// if atomic.AddUint32(&s.closing, 1) == 1 {
	// 	s.log.Info("Spawning process manager stopping")

	// 	// TODO
	// }

	// return nil
	panic("Not implemented")
}

func (s *SpawningProcessManager) Start(delegate AgentDelegate) error {
	s.delegate = delegate
	s.log.Info("Spawning (no sandbox) process manager starting")

	// // TODO

	// return nil
	panic("Not implemented")
}

func (s *SpawningProcessManager) StopProcess(agentID string) error {
	// TODO

	panic("Not implemented")
}

func (s *SpawningProcessManager) Lookup(agentID string) (*agentapi.DeployRequest, error) {
	// TODO

	panic("Not implemented")
}
