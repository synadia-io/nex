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
type ContactLostCallback func(string)

const (
	defaultAgentPingIntervalMillis = 5000

	NexTriggerSubject = "x-nex-trigger-subject"
	NexRuntimeNs      = "x-nex-runtime-ns"

	HttpURLHeader = "x-http-url"

	KeyValueKeyHeader = "x-keyvalue-key"

	MessagingSubjectHeader = "x-subject"

	ObjectStoreObjectNameHeader = "x-object-name"
)

type AgentClient struct {
	nc                *nats.Conn
	log               *slog.Logger
	agentID           string
	handshakeTimeout  time.Duration
	handshakeReceived *atomic.Bool
	pingTimeout       time.Duration
	stopping          uint32

	handshakeTimedOut  HandshakeCallback
	handshakeSucceeded HandshakeCallback
	eventReceived      EventCallback
	logReceived        LogCallback
	contactLost        ContactLostCallback

	execTotalNanos    int64
	workloadStartedAt time.Time

	workloadBytes uint64
	subz          []*nats.Subscription
}

func NewAgentClient(
	nc *nats.Conn,
	log *slog.Logger,
	handshakeTimeout, pingTimeout time.Duration,
	onTimedOut HandshakeCallback,
	onSuccess HandshakeCallback,
	onContactLost ContactLostCallback,
	onEvent EventCallback,
	onLog LogCallback,
) *AgentClient {
	return &AgentClient{
		contactLost:        onContactLost,
		eventReceived:      onEvent,
		handshakeReceived:  &atomic.Bool{},
		handshakeTimeout:   handshakeTimeout,
		handshakeTimedOut:  onTimedOut,
		handshakeSucceeded: onSuccess,
		log:                log,
		logReceived:        onLog,
		nc:                 nc,
		pingTimeout:        pingTimeout,
		subz:               make([]*nats.Subscription, 0),
	}
}

// Returns the ID of this agent client, which corresponds to a workload process identifier
func (a *AgentClient) ID() string {
	return a.agentID
}

// Agent client instances subscribe to the following `hostint.>` subjects,
// which are exported by the `nexnode` account on the configured internal
// NATS connection for consumption by agents:
//
// - hostint.<agent_id>.handshake
// - hostint.<agent_id>.events
// - hostint.<agent_id>.logs
func (a *AgentClient) Start(agentID string) error {
	a.log.Info("Agent client starting", slog.String("agent_id", agentID))
	a.agentID = agentID

	var sub *nats.Subscription
	var err error

	sub, err = a.nc.Subscribe(fmt.Sprintf("hostint.%s.handshake", agentID), a.handleHandshake)
	if err != nil {
		return err
	}
	a.subz = append(a.subz, sub)

	sub, err = a.nc.Subscribe(fmt.Sprintf("hostint.%s.events.*", agentID), a.handleAgentEvent)
	if err != nil {
		return err
	}
	a.subz = append(a.subz, sub)

	sub, err = a.nc.Subscribe(fmt.Sprintf("hostint.%s.logs", agentID), a.handleAgentLog)
	if err != nil {
		return err
	}
	a.subz = append(a.subz, sub)

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
		slog.String("agent_id", a.agentID),
		slog.String("status", status.String()))

	subject := fmt.Sprintf("agentint.%s.deploy", a.agentID)
	resp, err := a.nc.Request(subject, bytes, 10*time.Second)
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

	a.workloadStartedAt = time.Now().UTC()
	a.workloadBytes = uint64(request.TotalBytes)
	return &deployResponse, nil
}

func (a *AgentClient) WorkloadBytes() uint64 {
	return a.workloadBytes
}

// Draining subscriptions and release other resources associated
// with the agent client
func (a *AgentClient) Drain() error {
	for _, sub := range a.subz {
		err := sub.Drain()
		if err != nil {
			a.log.Warn("failed to drain subscription associated with agent client",
				slog.String("subject", sub.Subject),
				slog.String("agent_id", a.agentID),
				slog.String("error", err.Error()),
			)

			// no-op for now, try the next one... perhaps we should return the error here in the future?
		}

		a.log.Debug("drained subscription associated with agent client",
			slog.String("subject", sub.Subject),
			slog.String("agent_id", a.agentID),
		)
	}

	return nil
}

// Stop the agent client instance
// Currently this method simply sets the stopping flag, e.g., allowing any pending handshakes to be aborted
func (a *AgentClient) Stop() error {
	if atomic.AddUint32(&a.stopping, 1) == 1 {
		return nil
	}

	return errors.New("agent client already stopping")
}

func (a *AgentClient) Undeploy() error {
	_ = a.Stop()

	subject := fmt.Sprintf("agentint.%s.undeploy", a.agentID)

	a.log.Debug("sending undeploy request to agent via internal NATS connection",
		slog.String("subject", subject),
		slog.String("agent_id", a.agentID),
	)

	_, err := a.nc.Request(subject, []byte{}, 500*time.Millisecond) // FIXME-- allow this timeout to be configurable... 500ms is likely not enough
	if err != nil {
		a.log.Warn("request to undeploy workload via internal NATS connection failed", slog.String("agent_id", a.agentID), slog.String("error", err.Error()))
		return err
	}

	return nil
}

func (a *AgentClient) Ping() error {
	subject := fmt.Sprintf("agentint.%s.ping", a.agentID)
	// a.log.Debug("pinging agent", slog.String("subject", subject))

	_, err := a.nc.Request(subject, []byte{}, a.pingTimeout)
	if err != nil && !a.shuttingDown() {
		a.log.Warn("agent failed to respond to ping", slog.Any("error", err))
		return err
	}

	return nil
}

func (a *AgentClient) RecordExecTime(elapsedNanos int64) {
	atomic.AddInt64(&a.execTotalNanos, elapsedNanos)
}

func (a *AgentClient) ExecTimeNanos() int64 {
	return a.execTotalNanos
}

// Returns the time difference between now and when the agent started
func (a *AgentClient) UptimeMillis() time.Duration {
	return time.Since(a.workloadStartedAt)
}

func (a *AgentClient) RunTrigger(ctx context.Context, tracer trace.Tracer, subject string, data []byte) (*nats.Msg, error) {
	intmsg := nats.NewMsg(fmt.Sprintf("agentint.%s.trigger", a.agentID))
	intmsg.Header.Add(NexTriggerSubject, subject)
	intmsg.Data = data

	cctx, childSpan := tracer.Start(
		ctx,
		"internal request",
		trace.WithSpanKind(trace.SpanKindClient),
	)

	otel.GetTextMapPropagator().Inject(cctx, propagation.HeaderCarrier(intmsg.Header))

	resp, err := a.nc.RequestMsg(intmsg, time.Millisecond*10000) // FIXME-- make timeout configurable
	childSpan.End()

	return resp, err
}

func (a *AgentClient) awaitHandshake(agentID string) {
	timeoutAt := time.Now().UTC().Add(a.handshakeTimeout)

	handshakeOk := false
	for !handshakeOk && !a.shuttingDown() {
		if time.Now().UTC().After(timeoutAt) {
			a.handshakeTimedOut(agentID)
			return
		}

		handshakeOk = a.handshakeReceived.Load()
		time.Sleep(time.Millisecond * DefaultRunloopSleepTimeoutMillis)
	}
}

func (a *AgentClient) handleHandshake(msg *nats.Msg) {
	var req *HandshakeRequest
	err := json.Unmarshal(msg.Data, &req)
	if err != nil {
		a.log.Error("Failed to handle agent handshake", slog.String("agent_id", *req.ID), slog.String("message", *req.Message))
		return
	}

	a.log.Info("Received agent handshake", slog.String("agent_id", *req.ID), slog.String("message", *req.Message))

	resp, _ := json.Marshal(&HandshakeResponse{})

	err = msg.Respond(resp)
	if err != nil {
		a.log.Error("Failed to reply to agent handshake", slog.Any("err", err))
		return
	}

	a.handshakeReceived.Store(true)
	a.handshakeSucceeded(*req.ID)
	go a.monitorAgent()
}

func (a *AgentClient) monitorAgent() {
	ticker := time.NewTicker(time.Millisecond * defaultAgentPingIntervalMillis) // FIXME-- make configurable
	defer ticker.Stop()

	for !a.shuttingDown() {
		<-ticker.C
		err := a.Ping()
		if err != nil {
			if a.contactLost != nil {
				a.contactLost(a.agentID)
			}
			break
		}
	}
}

func (a *AgentClient) handleAgentEvent(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	agentID := tokens[1]

	var evt cloudevents.Event
	err := evt.UnmarshalJSON(msg.Data)
	if err != nil {
		a.log.Error("Failed to deserialize cloudevent from agent", slog.Any("err", err))
		return
	}

	a.log.Info("Received agent event", slog.String("agent_id", agentID), slog.String("type", evt.Type()))
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

	a.log.Debug("Received agent log", slog.String("agent_id", agentID), slog.String("log", logentry.Text))
	a.logReceived(agentID, logentry)
}

func (a *AgentClient) shuttingDown() bool {
	return (atomic.LoadUint32(&a.stopping) > 0)
}
