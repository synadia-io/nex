package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type HandshakeCallback func(string)
type EventCallback func(string, cloudevents.Event)
type LogCallback func(string, LogEntry)

const (
	nexTriggerSubject = "x-nex-trigger-subject"
)

type AgentClient struct {
	nc                *nats.Conn
	log               *slog.Logger
	agentID           string
	handshakeTimeout  time.Duration
	handshakeReceived *atomic.Bool

	handshakeTimedOut  HandshakeCallback
	handshakeSucceeded HandshakeCallback
	eventReceived      EventCallback
	logReceived        LogCallback
}

func NewAgentClient(
	nc *nats.Conn,
	log *slog.Logger,
	handshakeTimeout time.Duration,
	onTimedOut HandshakeCallback,
	onSuccess HandshakeCallback,
	onEvent EventCallback,
	onLog LogCallback,
) *AgentClient {
	return &AgentClient{
		eventReceived:      onEvent,
		handshakeReceived:  &atomic.Bool{},
		handshakeTimeout:   handshakeTimeout,
		handshakeTimedOut:  onTimedOut,
		handshakeSucceeded: onSuccess,
		log:                log,
		logReceived:        onLog,
		nc:                 nc,
	}
}

// Returns the ID of this agent client, which corresponds to a workload process identifier
func (a *AgentClient) ID() string {
	return a.agentID
}

func (a *AgentClient) Start(agentID string) error {
	a.log.Info("Agent client starting", slog.String("workload_id", agentID))
	a.agentID = agentID

	_, err := a.nc.Subscribe("agentint.handshake", a.handleHandshake)
	if err != nil {
		return err
	}

	_, err = a.nc.Subscribe(fmt.Sprintf("agentint.%s.events.*", agentID), a.handleAgentEvent)
	if err != nil {
		return err
	}

	_, err = a.nc.Subscribe(fmt.Sprintf("agentint.%s.logs", agentID), a.handleAgentLog)
	if err != nil {
		return err
	}

	go a.awaitHandshake(agentID)

	return nil
}

func (a *AgentClient) DeployWorkload(request *DeployRequest) (*DeployResponse, error) {
	bytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	status := a.nc.Status()
	a.log.Debug("NATS internal connection status",
		slog.String("workload_id", a.agentID),
		slog.String("status", status.String()))

	subject := fmt.Sprintf("agentint.%s.deploy", a.agentID)
	resp, err := a.nc.Request(subject, bytes, 1*time.Second)
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return nil, errors.New("timed out waiting for acknowledgement of workload deployment")
		} else {
			return nil, fmt.Errorf("failed to submit request for workload deployment: %s", err)
		}
	}

	var deployResponse DeployResponse
	err = json.Unmarshal(resp.Data, &deployResponse)
	if err != nil {
		a.log.Error("Failed to deserialize deployment response", slog.Any("error", err))
		return nil, err
	}
	return &deployResponse, nil
}

func (a *AgentClient) Undeploy() error {
	subject := fmt.Sprintf("agentint.%s.undeploy", a.agentID)
	_, err := a.nc.Request(subject, []byte{}, 500*time.Millisecond) // FIXME-- allow this timeout to be configurable... 500ms is likely not enough
	if err != nil {
		a.log.Warn("request to undeploy workload via internal NATS connection failed", slog.String("workload_id", a.agentID), slog.String("error", err.Error()))
		return err
	}
	return nil
}

func (a *AgentClient) RunTrigger(ctx context.Context, tracer trace.Tracer, subject string, data []byte) (*nats.Msg, error) {
	intmsg := nats.NewMsg(fmt.Sprintf("agentint.%s.trigger", a.agentID))
	// TODO: inject tracer context into message header
	intmsg.Header.Add(nexTriggerSubject, subject)
	intmsg.Data = data

	cctx, childSpan := tracer.Start(
		ctx,
		"internal request",
		trace.WithSpanKind(trace.SpanKindClient),
	)

	otel.GetTextMapPropagator().Inject(cctx, propagation.HeaderCarrier(intmsg.Header))

	// TODO: make the agent's exec handler extract and forward the otel context
	// so it continues in the host services like kv, obj, msg, etc
	resp, err := a.nc.RequestMsg(intmsg, time.Millisecond*10000) // FIXME-- make timeout configurable
	childSpan.End()

	return resp, err
}

func (a *AgentClient) awaitHandshake(agentID string) {
	<-time.After(a.handshakeTimeout)
	if !a.handshakeReceived.Load() {
		a.handshakeTimedOut(agentID)
	}
}

func (a *AgentClient) handleHandshake(msg *nats.Msg) {
	var req *HandshakeRequest
	err := json.Unmarshal(msg.Data, &req)
	if err != nil {
		a.log.Error("Failed to handle agent handshake", slog.String("workload_id", *req.MachineID), slog.String("message", *req.Message))
		return
	}

	a.log.Info("Received agent handshake", slog.String("workload_id", *req.MachineID), slog.String("message", *req.Message))

	resp, _ := json.Marshal(&HandshakeResponse{})

	err = msg.Respond(resp)
	if err != nil {
		a.log.Error("Failed to reply to agent handshake", slog.Any("err", err))
		return
	}

	a.handshakeReceived.Store(true)
	a.handshakeSucceeded(*req.MachineID)
}

func (a *AgentClient) handleAgentEvent(msg *nats.Msg) {
	// agentint.{agentID}.events.{type}
	tokens := strings.Split(msg.Subject, ".")
	agentID := tokens[1]

	var evt cloudevents.Event
	err := json.Unmarshal(msg.Data, &evt)
	if err != nil {
		a.log.Error("Failed to deserialize cloudevent from agent", slog.Any("err", err))
		return
	}

	a.log.Info("Received agent event", slog.String("workload_id", agentID), slog.String("type", evt.Type()))
	a.eventReceived(agentID, evt)
}

func (a *AgentClient) handleAgentLog(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	agentID := tokens[1]

	var logentry LogEntry
	err := json.Unmarshal(msg.Data, &logentry)
	if err != nil {
		a.log.Error("Failed to unmarshal log entry from agent", slog.Any("err", err))
		return
	}

	a.log.Debug("Received agent log", slog.String("workload_id", agentID), slog.String("log", logentry.Text))
	a.logReceived(agentID, logentry)
}
