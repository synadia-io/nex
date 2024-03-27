package agentapi

import (
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
)

type HandshakeCallback func(string)
type EventCallback func(string, cloudevents.Event)
type LogCallback func(string, LogEntry)

type AgentClient struct {
	nc                *nats.Conn
	log               *slog.Logger
	agentId           string
	handshakeTimeout  time.Duration
	handshakeReceived *atomic.Bool

	handshakeTimedOut  HandshakeCallback
	handshakeSucceeded HandshakeCallback
	eventReceived      EventCallback
	logReceived        LogCallback
}

func NewAgentClient(nc *nats.Conn,
	handshakeTimeout time.Duration,
	onTimedOut HandshakeCallback,
	onSuccess HandshakeCallback,
	onEvent EventCallback,
	onLog LogCallback,
	log *slog.Logger,
) *AgentClient {
	shake := &atomic.Bool{}
	shake.Store(false)
	return &AgentClient{
		nc:                 nc,
		handshakeTimeout:   handshakeTimeout,
		handshakeTimedOut:  onTimedOut,
		handshakeSucceeded: onSuccess,
		eventReceived:      onEvent,
		logReceived:        onLog,

		log:               log,
		handshakeReceived: shake,
	}
}

func (a *AgentClient) Id() string {
	return a.agentId
}

func (a *AgentClient) Start(agentId string) error {
	a.agentId = agentId
	_, err := a.nc.Subscribe("agentint.handshake", a.handleHandshake)
	if err != nil {
		return err
	}

	_, err = a.nc.Subscribe(fmt.Sprintf("agentint.%s.events.*", agentId), a.handleAgentEvent)
	if err != nil {
		return err
	}

	_, err = a.nc.Subscribe(fmt.Sprintf("agentint.%s.logs", agentId), a.handleAgentLog)
	if err != nil {
		return err
	}

	go a.awaitHandshake(agentId)

	return nil
}

func (a *AgentClient) DeployWorkload(request *DeployRequest) (*DeployResponse, error) {
	bytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	status := a.nc.Status()
	a.log.Debug("NATS internal connection status",
		slog.String("agentId", a.agentId),
		slog.String("status", status.String()))

	subject := fmt.Sprintf("agentint.%s.deploy", a.agentId)
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
	subject := fmt.Sprintf("agentint.%s.undeploy", a.agentId)
	_, err := a.nc.Request(subject, []byte{}, 500*time.Millisecond) // FIXME-- allow this timeout to be configurable... 500ms is likely not enough
	if err != nil {
		a.log.Warn("request to undeploy workload via internal NATS connection failed",
			slog.String("agentId", a.agentId), slog.String("error", err.Error()))
		return err
	}
	return nil
}

func (a *AgentClient) awaitHandshake(agentId string) {
	<-time.After(a.handshakeTimeout)
	if !a.handshakeReceived.Load() {
		a.handshakeTimedOut(agentId)
	}
}

func (a *AgentClient) handleHandshake(msg *nats.Msg) {
	var req HandshakeRequest
	err := json.Unmarshal(msg.Data, &req)
	if err != nil {
		a.log.Error("Failed to handle agent handshake", slog.String("agentId", *req.MachineID), slog.String("message", *req.Message))
		return
	}

	a.log.Info("Received agent handshake", slog.String("agentId", *req.MachineID), slog.String("message", *req.Message))

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
	// agentint.{agentId}.events.{type}
	tokens := strings.Split(msg.Subject, ".")
	agentId := tokens[1]

	var evt cloudevents.Event
	err := json.Unmarshal(msg.Data, &evt)
	if err != nil {
		a.log.Error("Failed to deserialize cloudevent from agent", slog.Any("err", err))
		return
	}

	a.log.Info("Received agent event", slog.String("agentId", agentId), slog.String("type", evt.Type()))
	a.eventReceived(agentId, evt)
}

func (a *AgentClient) handleAgentLog(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	agentId := tokens[1]

	var logentry LogEntry
	err := json.Unmarshal(msg.Data, &logentry)
	if err != nil {
		a.log.Error("Failed to unmarshal log entry from agent", slog.Any("err", err))
		return
	}

	a.log.Debug("Received agent log", slog.String("agentId", agentId), slog.String("log", logentry.Text))
	a.logReceived(agentId, logentry)
}
