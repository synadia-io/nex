package agent

import (
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/synadia-labs/nex/models"
)

type agentLogCapture struct {
	nc         *nats.Conn
	logger     *slog.Logger
	stderr     bool
	agentId    string
	workloadId string
	namespace  string
}

func NewAgentLogCapture(
	nc *nats.Conn, logger *slog.Logger, stderr bool, agentId string, workloadId string, namespace string,
) *agentLogCapture {
	return &agentLogCapture{
		nc:         nc,
		logger:     logger,
		stderr:     stderr,
		agentId:    agentId,
		workloadId: workloadId,
		namespace:  namespace,
	}
}

func (l agentLogCapture) Write(p []byte) (n int, err error) {
	if l.stderr {
		l.logger.WithGroup("workload").With("agent", l.agentId).With("workload", l.workloadId).Error(strings.ReplaceAll(string(p), "\n", "\\n"))
	} else {
		l.logger.WithGroup("workload").With("agent", l.agentId).With("workload", l.workloadId).Info(strings.ReplaceAll(string(p), "\n", "\\n"))
	}

	err = l.nc.Publish(models.AgentEmitLogSubject(l.namespace, l.workloadId), p)
	if err != nil {
		slog.Error("Failed to publish log message to nats", slog.Any("err", err), slog.Any("workload", l.workloadId))
	}
	return len(p), nil
}
