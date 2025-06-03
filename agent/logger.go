package agent

import (
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/synadia-labs/nex/models"
)

type LogType int

type agentLogCapture struct {
	nc         *nats.Conn
	logger     *slog.Logger
	logType    models.LogOut
	agentId    string
	workloadId string
	namespace  string
}

func NewAgentLogCapture(
	nc *nats.Conn, logger *slog.Logger, logType models.LogOut, agentId string, workloadId string, namespace string,
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
	var msg *nats.Msg
	switch l.logType {
	case models.LogOutStdout:
		l.logger.WithGroup("workload").With("agent", l.agentId).With("workload", l.workloadId).Info(strings.ReplaceAll(string(p), "\n", "\\n"))
		msg = nats.NewMsg(models.AgentEmitLogSubject(l.namespace, l.workloadId, models.LogOutStdout))
	case models.LogOutStderr:
		l.logger.WithGroup("workload").With("agent", l.agentId).With("workload", l.workloadId).Error(strings.ReplaceAll(string(p), "\n", "\\n"))
		msg = nats.NewMsg(models.AgentEmitLogSubject(l.namespace, l.workloadId, models.LogOutStderr))
	case models.LogOutMetrics:
		msg = nats.NewMsg(models.AgentEmitMetricsSubject(l.namespace, l.workloadId))
	}

	msg.Header.Add(models.NexGroupMetaKey, "") // TODO: add group label
	msg.Header.Add(models.NexNamespaceMetaKey, l.namespace)
	msg.Data = p

	err = l.nc.PublishMsg(msg)
	if err != nil {
		slog.Error("Failed to publish log message to nats", slog.String("err", err.Error()), slog.Any("workload", l.workloadId))
	}
	return len(p), nil
}
