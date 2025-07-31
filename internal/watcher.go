package internal

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nexlet.go/agent"
	"github.com/synadia-labs/nex/internal/emitter"
	"github.com/synadia-labs/nex/models"
)

type AgentProcess struct {
	Config  *models.Agent
	Process *os.Process

	ID       string
	HostNode string

	initialized  bool
	agentLock    sync.Mutex
	restartCount int
	state        models.AgentState
}

// AgentWatcher is responsible for managing the lifecycle of agents, including
// starting, stopping, and restarting them as needed.
type AgentWatcher struct {
	ctx         context.Context
	nc          *nats.Conn
	nodeKeypair nkeys.KeyPair
	logger      *slog.Logger

	initAgentsWg *sync.WaitGroup
	resetLimit   int

	embeddedAgents     map[string]*agent.Runner // maps agentID to agent Runner
	embeddedAgentsLock sync.Mutex

	localAgents    map[string]*AgentProcess // maps agentID to AgentProcess
	localAgentLock sync.Mutex               // Lock for managing access to localAgents

	agentCount int
}

func NewAgentWatcher(ctx context.Context, nc *nats.Conn, kp nkeys.KeyPair, logger *slog.Logger, restarts int, wg *sync.WaitGroup) *AgentWatcher {
	return &AgentWatcher{
		ctx:          ctx,
		nc:           nc,
		nodeKeypair:  kp,
		logger:       logger,
		resetLimit:   restarts,
		initAgentsWg: wg,
		agentCount:   0,

		embeddedAgents:     make(map[string]*agent.Runner),
		embeddedAgentsLock: sync.Mutex{},
		localAgents:        make(map[string]*AgentProcess),
		localAgentLock:     sync.Mutex{},
	}
}

func (a *AgentWatcher) WaitForAgents() {
	if a.initAgentsWg != nil {
		a.initAgentsWg.Wait()
	}
}

func (a *AgentWatcher) Shutdown() {
	a.logger.Info("shutting down agent watcher", slog.Int("agent_count", a.agentCount))

	// Stop all embedded agents
	for agentID := range a.embeddedAgents {
		if err := a.StopEmbeddedAgent(agentID); err != nil {
			a.logger.Error("failed to stop embedded agent", slog.String("agent_id", agentID), slog.String("err", err.Error()))
		}
		delete(a.embeddedAgents, agentID)
	}
	for agentID, ap := range a.localAgents {
		// Stop all local agents
		if ap.Process != nil {
			err := a.StopLocalBinaryAgent(agentID)
			if err != nil {
				a.logger.Error("failed to stop agent running as local process", slog.String("agent_id", agentID), slog.String("err", err.Error()))
			}
		}
		delete(a.localAgents, agentID)
	}

	a.logger.Info("agent watcher shutdown complete")
}

func (a *AgentWatcher) StartEmbeddedAgent(agentID string, runner *agent.Runner, connData *models.NatsConnectionData) {
	restartCount := 0

	for range a.resetLimit {
		err := runner.Run(agentID, *connData)
		if err != nil {
			restartCount++
			if restartCount <= a.resetLimit {
				a.logger.Info("restarting agent", slog.String("agent_name", runner.String()), slog.Int("restart_count", restartCount), slog.Int("reset_limit", a.resetLimit))
				continue
			} else {
				a.logger.Error("agent failed to start after maximum retries", slog.String("agent_name", runner.String()), slog.Int("reset_limit", a.resetLimit), slog.String("err", err.Error()))
				a.initAgentsWg.Done()
				return
			}
		}
		break
	}

	a.embeddedAgentsLock.Lock()
	a.embeddedAgents[agentID] = runner
	a.embeddedAgentsLock.Unlock()

	a.logger.Info("started embedded agent", slog.String("agent_name", runner.String()))
	a.agentCount++
	a.initAgentsWg.Done()
}

func (a *AgentWatcher) StopEmbeddedAgent(agentID string) error {
	a.embeddedAgentsLock.Lock()
	runner, exists := a.embeddedAgents[agentID]
	a.embeddedAgentsLock.Unlock()

	if !exists {
		a.logger.Warn("attempted to stop non-existent embedded agent", slog.String("agent_id", agentID))
		return errors.New("embedded agent not found")
	}

	err := runner.Shutdown()
	if err != nil {
		a.logger.Error("failed to stop embedded agent", slog.String("agent_id", agentID), slog.String("err", err.Error()))
		return err
	}

	a.logger.Debug("stopped embedded agent", slog.String("agent_id", agentID))
	a.agentCount--

	err = emitter.EmitSystemEvent(a.nc, a.nodeKeypair, &models.AgentStoppedEvent{
		Id:        agentID,
		Timestamp: time.Now(),
	})
	if err != nil {
		a.logger.Error("failed to emit agent stopped event", slog.String("agent_id", agentID), slog.String("err", err.Error()))
	}

	return nil
}

func (a *AgentWatcher) StartLocalBinaryAgent(ap *AgentProcess, regCreds *models.NatsConnectionData) {
	fPath := strings.TrimPrefix(ap.Config.Uri, "file://")
	info, err := os.Stat(fPath)
	if err != nil || info.IsDir() {
		a.logger.Error("provide path is not a binary file", slog.String("agent_uri", ap.Config.Uri), slog.String("err", err.Error()))
		a.initAgentsWg.Done()
		return
	}

	for ap.restartCount <= a.resetLimit {
		select {
		case <-a.ctx.Done():
			if !ap.initialized {
				a.initAgentsWg.Done()
			}
			return
		default:
			ap.agentLock.Lock()

			env := []string{}
			for k, v := range ap.Config.Env {
				env = append(env, k+"="+v)
			}

			env = append(env, []string{
				"NEX_AGENT_NATS_SERVERS=" + strings.Join(regCreds.NatsServers, ","),
				"NEX_AGENT_NATS_USER_SEED=" + regCreds.NatsUserSeed,
				"NEX_AGENT_NATS_B64_JWT=" + base64.StdEncoding.EncodeToString([]byte(regCreds.NatsUserJwt)),
				"NEX_AGENT_NATS_USER=" + regCreds.NatsUserName,
				"NEX_AGENT_NATS_PASSWORD=" + regCreds.NatsUserPassword,
				"NEX_AGENT_NATS_USER_NKEY=" + regCreds.NatsUserNkey,
				"NEX_AGENT_NODE_ID=" + ap.HostNode,
				"NEX_AGENT_ASSIGNED_ID=" + ap.ID,
			}...)

			// Start the process
			cmd := exec.CommandContext(a.ctx, fPath, ap.Config.Argv...)
			cmd.Env = env
			cmd.Stdout = agentLogCapture{logger: a.logger.WithGroup(info.Name()).With("agent_id", ap.ID), stderr: false}
			cmd.Stderr = agentLogCapture{logger: a.logger.WithGroup(info.Name()).With("agent_id", ap.ID), stderr: true}
			cmd.SysProcAttr = SysProcAttr()

			err = cmd.Start()
			if err != nil {
				a.logger.Error("failed to start local agent", slog.String("agent", info.Name()), slog.String("err", err.Error()))
				ap.restartCount++
				ap.agentLock.Unlock()
				continue
			}

			ap.Process = cmd.Process
			a.logger.Info("started local agent", slog.String("agent", info.Name()), slog.Int("restart_count", ap.restartCount), slog.Int("reset_limit", a.resetLimit), slog.Int("pid", ap.Process.Pid))

			if !ap.initialized {
				a.initAgentsWg.Done()
			}
			ap.initialized = true
			a.agentCount++

			a.localAgentLock.Lock()
			a.localAgents[ap.ID] = ap
			a.localAgentLock.Unlock()

			state, err := cmd.Process.Wait()
			if err != nil {
				a.logger.Error("Nexlet process exited with error", slog.Int("process", ap.Process.Pid), slog.String("err", err.Error()))
			} else if state != nil && ap.state != models.AgentStateStopping || ap.state != models.AgentStateLameduck {
				a.logger.Warn("Nexlet process unexpectedly exited with state", slog.Any("state", state), slog.Int("process", ap.Process.Pid))
			}

			a.agentCount--

			if ap.state == models.AgentStateStopping || ap.state == models.AgentStateLameduck {
				break
			}

			ap.restartCount++
			ap.agentLock.Unlock()
		}
	}
}

func (a *AgentWatcher) StopLocalBinaryAgent(agentID string) error {
	a.localAgentLock.Lock()
	ap, exists := a.localAgents[agentID]
	a.localAgentLock.Unlock()

	if !exists {
		a.logger.Warn("attempted to stop non-existent local agent", slog.String("agent_id", agentID))
		return errors.New("local agent not found")
	}

	ap.agentLock.Lock()
	defer ap.agentLock.Unlock()

	ap.state = models.AgentStateStopping

	if ap.Process == nil {
		a.logger.Warn("local agent process is nil, cannot stop", slog.String("agent_id", agentID))
		return errors.New("local agent process not running")
	}

	err := ap.Process.Signal(os.Interrupt)
	if err != nil {
		a.logger.Error("failed to stop local agent", slog.String("agent_id", agentID), slog.String("err", err.Error()))
		return err
	}

	a.logger.Debug("stopped local agent", slog.String("agent_id", agentID))

	err = emitter.EmitSystemEvent(a.nc, a.nodeKeypair, &models.AgentStoppedEvent{
		Id:        agentID,
		Timestamp: time.Now(),
	})
	if err != nil {
		a.logger.Error("failed to emit agent stopped event", slog.String("agent_id", agentID), slog.String("err", err.Error()))
	}
	return nil
}
