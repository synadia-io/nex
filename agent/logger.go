package agent

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/synadia-labs/nex/models"
)

type LogType int

func (lt LogType) String() string {
	switch lt {
	case LogTypeStdout:
		return "stdout"
	case LogTypeStderr:
		return "stderr"
	case LogTypeMetrics:
		return "metrics"
	default:
		return "unknown"
	}
}

const (
	LogTypeStdout LogType = iota
	LogTypeStderr
	LogTypeMetrics
)

type agentLogCapture struct {
	nc         *nats.Conn
	logger     *slog.Logger
	logType    LogType
	agentId    string
	workloadId string
	namespace  string
}

func NewAgentLogCapture(
	nc *nats.Conn, logger *slog.Logger, logType LogType, agentId string, workloadId string, namespace string,
) *agentLogCapture {
	return &agentLogCapture{
		nc:         nc,
		logger:     logger,
		logType:    logType,
		agentId:    agentId,
		workloadId: workloadId,
		namespace:  namespace,
	}
}

func (l agentLogCapture) Write(p []byte) (n int, err error) {
	switch l.logType {
	case LogTypeStdout:
		l.logger.WithGroup("workload").With("agent", l.agentId).With("workload", l.workloadId).Info(strings.ReplaceAll(string(p), "\n", "\\n"))
	case LogTypeStderr:
		l.logger.WithGroup("workload").With("agent", l.agentId).With("workload", l.workloadId).Error(strings.ReplaceAll(string(p), "\n", "\\n"))
	}

	err = l.nc.Publish(fmt.Sprintf("%s.%s", models.AgentEmitLogSubject(l.namespace, l.workloadId), l.logType.String()), p)
	if err != nil {
		slog.Error("Failed to publish log message to nats", slog.Any("err", err), slog.Any("workload", l.workloadId))
	}
	return len(p), nil
}
