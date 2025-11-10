package agent

import (
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/models"
)

type LogType int

type agentLogCapture struct {
	nc         *nats.Conn
	logger     *slog.Logger
	logType    models.LogOut
	agentID    string
	workloadID string
	namespace  string
}

func NewAgentLogCapture(
	nc *nats.Conn, logger *slog.Logger, logType models.LogOut, agentID string, workloadID string, namespace string,
) *agentLogCapture {
	return &agentLogCapture{
		nc:         nc,
		logger:     logger,
		logType:    logType,
		agentID:    agentID,
		workloadID: workloadID,
		namespace:  namespace,
	}
}

func (l agentLogCapture) Write(p []byte) (n int, err error) {
	var msg *nats.Msg
	switch l.logType {
	case models.LogOutStdout:
		l.logger.WithGroup("workload").With("agent", l.agentID).With("workload", l.workloadID).Info(strings.ReplaceAll(string(p), "\n", "\\n"))
		msg = nats.NewMsg(models.AgentEmitLogSubject(l.namespace, l.workloadID, models.LogOutStdout))
	case models.LogOutStderr:
		l.logger.WithGroup("workload").With("agent", l.agentID).With("workload", l.workloadID).Error(strings.ReplaceAll(string(p), "\n", "\\n"))
		msg = nats.NewMsg(models.AgentEmitLogSubject(l.namespace, l.workloadID, models.LogOutStderr))
	case models.LogOutMetrics:
		msg = nats.NewMsg(models.AgentEmitMetricsSubject(l.namespace, l.workloadID))
	}

	msg.Header.Add(models.NexGroupMetaKey, "") // TODO: add group label
	msg.Header.Add(models.NexNamespaceMetaKey, l.namespace)
	msg.Data = p

	err = l.nc.PublishMsg(msg)
	if err != nil {
		slog.Error("Failed to publish log message to nats", slog.String("err", err.Error()), slog.Any("workload", l.workloadID))
	}
	return len(p), nil
}
