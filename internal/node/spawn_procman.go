package nexnode

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/xid"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const (
	nexAgentBinary = "nex-agent"
)

// A process manager that controls the creation and deletion of `nex-agent` processes, directly
// spawned as children of the nex node
type SpawningProcessManager struct {
	closing     uint32
	config      *NodeConfiguration
	ctx         context.Context
	stopMutexes map[string]*sync.Mutex
	t           *Telemetry

	liveProcs map[string]*spawnedProcess
	warmProcs chan *spawnedProcess

	delegate       ProcessDelegate
	deployRequests map[string]*agentapi.DeployRequest

	log *slog.Logger
}

type spawnedProcess struct {
	cmd             *exec.Cmd
	deployRequest   *agentapi.DeployRequest
	workloadStarted time.Time

	ID string

	Fail chan bool
	Run  chan bool
	Exit chan int

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

		stopMutexes: make(map[string]*sync.Mutex),

		deployRequests: make(map[string]*agentapi.DeployRequest),
		liveProcs:      make(map[string]*spawnedProcess),
		warmProcs:      make(chan *spawnedProcess, config.MachinePoolSize),
	}, nil
}

// Returns the list of processes that have been associated with a workload via deploy request
func (s *SpawningProcessManager) ListProcesses() ([]ProcessInfo, error) {
	pinfos := make([]ProcessInfo, 0)

	for workloadID, proc := range s.liveProcs {
		// Ignore pending "unprepared" processes that don't have workloads on them yet
		if proc.deployRequest != nil {
			pinfo := ProcessInfo{
				ID:            workloadID,
				Name:          *proc.deployRequest.WorkloadName,
				Namespace:     *proc.deployRequest.Namespace,
				DeployRequest: proc.deployRequest,
			}
			pinfos = append(pinfos, pinfo)
		}
	}

	return pinfos, nil
}

// Attaches a deployment request to a running process. Until a process is prepared, it's just an empty agent
func (s *SpawningProcessManager) PrepareWorkload(workloadID string, deployRequest *agentapi.DeployRequest) error {
	select {
	case proc := <-s.warmProcs:
		if proc == nil {
			return fmt.Errorf("could not prepare workload, no agent process")
		}
		proc.deployRequest = deployRequest
		proc.workloadStarted = time.Now().UTC()

		s.deployRequests[proc.ID] = deployRequest
	case <-time.After(500 * time.Millisecond):
		return fmt.Errorf("timed out waiting for available agent process")
	}

	return nil
}

// Stops the entire process manager. Called by the workload manager, typically via signal capture
func (s *SpawningProcessManager) Stop() error {
	if atomic.AddUint32(&s.closing, 1) == 1 {
		s.log.Info("Spawning process manager stopping")
		close(s.warmProcs)

		for workloadID := range s.liveProcs {
			err := s.StopProcess(workloadID)
			if err != nil {
				s.log.Warn("Failed to stop spawned process", slog.String("workload_id", workloadID), slog.String("error", err.Error()))
			}
		}
	}

	return nil
}

// Starts the process manager and creates the spawn loop for agent instances in the pool
func (s *SpawningProcessManager) Start(delegate ProcessDelegate) error {
	s.delegate = delegate
	s.log.Info("Spawning (no sandbox) process manager starting")

	for !s.stopping() {
		select {
		case <-s.ctx.Done():
			return nil
		default:
			if len(s.warmProcs) == s.config.MachinePoolSize {
				time.Sleep(runloopSleepInterval)
				continue
			}

			p, err := s.spawn()
			if err != nil {
				s.log.Error("Failed to spawn nex-agent for pool", slog.Any("error", err))
			}

			s.liveProcs[p.ID] = p
			s.stopMutexes[p.ID] = &sync.Mutex{}

			go s.delegate.OnProcessStarted(p.ID)

			s.log.Info("Adding new agent process to warm pool",
				slog.String("workload_id", p.ID))

			s.warmProcs <- p // If the pool is full, this line will block until a slot is available.
		}
	}

	return nil
}

// Stops a single agent process
func (s *SpawningProcessManager) StopProcess(workloadID string) error {
	proc, exists := s.liveProcs[workloadID]
	if !exists {
		return fmt.Errorf("failed to stop process %s. No such process", workloadID)
	}

	delete(s.deployRequests, workloadID)

	mutex := s.stopMutexes[workloadID]
	mutex.Lock()
	defer mutex.Unlock()

	s.log.Debug("Attempting to stop process", slog.String("workload_id", workloadID))

	s.kill(proc)

	delete(s.liveProcs, workloadID)
	delete(s.stopMutexes, workloadID)

	return nil
}

// Looks up an agent process. A non-existent agent process returns (nil, nil), not
// an error
func (s *SpawningProcessManager) Lookup(workloadID string) (*agentapi.DeployRequest, error) {
	if request, ok := s.deployRequests[workloadID]; ok {
		return request, nil
	}

	// Per contract, a non-prepared workload returns nil, not error
	return nil, nil
}

// Checks if the process manager is stopping
func (s *SpawningProcessManager) stopping() bool {
	return (atomic.LoadUint32(&s.closing) > 0)
}

// Spawns a new child process, a waiting nex-agent
func (s *SpawningProcessManager) spawn() (*spawnedProcess, error) {
	id := xid.New()
	workloadID := id.String()

	cmd := exec.Command(nexAgentBinary)
	cmd.Env = append(os.Environ(),
		"NEX_SANDBOX=false",
		fmt.Sprintf("NEX_WORKLOADID=%s", workloadID),
		// can't use the CNI host because we don't use it in no-sandbox mode
		"NEX_NODE_NATS_HOST=0.0.0.0",
		fmt.Sprintf("NEX_NODE_NATS_PORT=%d", *s.config.InternalNodePort),
	)

	cmd.Stderr = &procLogEmitter{workloadID: workloadID, log: s.log, stderr: true}
	cmd.Stdout = &procLogEmitter{workloadID: workloadID, log: s.log, stderr: false}

	newProc := &spawnedProcess{
		ID:   workloadID,
		cmd:  cmd,
		log:  s.log,
		Fail: make(chan bool),
		Run:  make(chan bool),
		Exit: make(chan int),
	}

	go func() {
		err := cmd.Run()
		if err != nil {
			s.log.Info("Command finished with error", slog.Any("error", err))
		}
	}()

	return newProc, nil
}

func (s *SpawningProcessManager) kill(proc *spawnedProcess) error {
	if proc.cmd.Process != nil {
		err := proc.cmd.Process.Signal(os.Kill)
		if err != nil {
			s.log.Error("Failed to kill OS process",
				slog.String("workload_id", proc.ID),
				slog.Int("pid", proc.cmd.Process.Pid))
			return err
		}
	}

	return nil
}

type procLogEmitter struct {
	stderr bool
	// TODO: personal opinion - not sure I like propagating logger instances everywhere...
	log        *slog.Logger
	workloadID string
}

// This function makes our procLogEmitter struct conform to the interface needed to capture
// stdout and stderr from a Cmd
func (l *procLogEmitter) Write(bytes []byte) (int, error) {
	msg := string(bytes)
	msg = strings.TrimSpace(msg)
	msg = strings.ReplaceAll(msg, "\n", "")

	if l.stderr {
		l.log.Error(msg, slog.String("workload_id", l.workloadID), slog.Bool("from_agent", true))
	} else {
		l.log.Info(msg, slog.String("workload_id", l.workloadID), slog.Bool("from_agent", true))
	}

	return len(bytes), nil
}
