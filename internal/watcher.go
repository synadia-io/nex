package internal

import (
	"context"
	"encoding/base64"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/synadia-labs/nex/models"
)

type AgentWatcher struct {
	ctx        context.Context
	logger     *slog.Logger
	resetLimit int
}

func NewAgentWatcher(ctx context.Context, logger *slog.Logger, restarts int) *AgentWatcher {
	return &AgentWatcher{
		ctx:        ctx,
		logger:     logger,
		resetLimit: restarts,
	}
}

func (a *AgentWatcher) New(regs *models.Regs, ap *AgentProcess, regCreds *models.NatsConnectionData) {
	fPath := strings.TrimPrefix(ap.Config.Uri, "file://")
	info, err := os.Stat(fPath)
	if err != nil || info.IsDir() {
		a.logger.Error("provide path is not a binary file", slog.String("agent_uri", ap.Config.Uri), slog.String("err", err.Error()))
		return
	}

	var mu sync.Mutex

	for ap.restartCount <= a.resetLimit {
		select {
		case <-a.ctx.Done():
			return
		default:
			mu.Lock()
			regs.New(ap.Id, models.RegTypeLocalAgent)

			env := []string{}
			for k, v := range ap.Config.Env {
				env = append(env, k+"="+v)
			}
			env = append(env, []string{
				"NEX_AGENT_NATS_URL=" + regCreds.NatsUrl,
				"NEX_AGENT_NATS_NKEY=" + regCreds.NatsUserSeed,
				"NEX_AGENT_NATS_B64_JWT=" + base64.StdEncoding.EncodeToString([]byte(regCreds.NatsUserJwt)),
				"NEX_AGENT_NODE_ID=" + ap.HostNode,
			}...)

			// Start the process
			cmd := exec.Command(fPath, ap.Config.Argv...)
			cmd.Env = env
			cmd.Stdout = agentLogCapture{logger: a.logger.WithGroup(info.Name()).With("agent_id", ap.Id), stderr: false}
			cmd.Stderr = agentLogCapture{logger: a.logger.WithGroup(info.Name()).With("agent_id", ap.Id), stderr: true}
			cmd.SysProcAttr = SysProcAttr()

			err = cmd.Start()
			if err != nil {
				a.logger.Error("failed to start local agent", slog.String("agent", info.Name()), slog.String("err", err.Error()))
				ap.restartCount++
				regs.Remove(ap.Id)
				mu.Unlock()
				continue
			}

			ap.Process = cmd.Process
			a.logger.Info("starting local agent", slog.String("agent", info.Name()), slog.Int("restart_count", ap.restartCount), slog.Int("reset_limit", a.resetLimit), slog.Int("pid", ap.Process.Pid))

			time.Sleep(250 * time.Millisecond)

			state, err := cmd.Process.Wait()
			if err != nil {
				a.logger.Error("Process exited with error", slog.Int("process", ap.Process.Pid), slog.Any("err", err))
			} else if state != nil && ap.state != models.AgentStateStopping || ap.state != models.AgentStateLameduck {
				a.logger.Warn("Process unexpectedly exited with state", slog.Any("state", state), slog.Int("process", ap.Process.Pid))
			}

			regs.Remove(ap.Id)

			if ap.state == models.AgentStateStopping || ap.state == models.AgentStateLameduck {
				break
			}

			ap.restartCount++
			mu.Unlock()
		}
	}
}
