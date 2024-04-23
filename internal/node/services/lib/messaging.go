package lib

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const messagingServiceMethodPublish = "publish"
const messagingServiceMethodRequest = "request"
const messagingServiceMethodRequestMany = "requestMany"

const messagingRequestTimeout = time.Millisecond * 500 // FIXME-- make timeout configurable per request?
const messagingRequestManyTimeout = time.Millisecond * 3000

const messagingSubjectHeader = "x-subject"

type MessagingService struct {
	log *slog.Logger
	nc  *nats.Conn
}

func NewMessagingService(nc *nats.Conn, log *slog.Logger) (*MessagingService, error) {
	messaging := &MessagingService{
		log: log,
		nc:  nc,
	}

	err := messaging.init()
	if err != nil {
		return nil, err
	}

	return messaging, nil
}

func (m *MessagingService) init() error {
	return nil
}

func (m *MessagingService) HandleRPC(msg *nats.Msg) {
	// agentint.{vmID}.rpc.{namespace}.{workload}.{service}.{method}
	tokens := strings.Split(msg.Subject, ".")
	service := tokens[5]
	method := tokens[6]

	switch method {
	case messagingServiceMethodPublish:
		m.handlePublish(msg)
	case messagingServiceMethodRequest:
		m.handleRequest(msg)
	case messagingServiceMethodRequestMany:
		m.handleRequestMany(msg)
	default:
		m.log.Warn("Received invalid host services RPC request",
			slog.String("service", service),
			slog.String("method", method),
		)
	}
}

func (m *MessagingService) handlePublish(msg *nats.Msg) {
	subject := msg.Header.Get(messagingSubjectHeader)
	if subject == "" {
		resp, _ := json.Marshal(&agentapi.HostServicesMessagingResponse{
			Errors: []string{"subject is required"},
		})

		err := msg.Respond(resp)
		if err != nil {
			m.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	err := m.nc.Publish(subject, msg.Data)
	if err != nil {
		m.log.Warn(fmt.Sprintf("failed to publish %d-byte message on subject %s: %s", len(msg.Data), subject, err.Error()))

		resp, _ := json.Marshal(&agentapi.HostServicesMessagingResponse{
			Errors: []string{fmt.Sprintf("failed to publish %d-byte message on subject %s: %s", len(msg.Data), subject, err.Error())},
		})

		err := msg.Respond(resp)
		if err != nil {
			m.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	resp, _ := json.Marshal(&agentapi.HostServicesMessagingResponse{
		Success: true,
	})
	err = msg.Respond(resp)
	if err != nil {
		m.log.Warn(fmt.Sprintf("failed to respond to messaging host service request: %s", err.Error()))
	}
}

func (m *MessagingService) handleRequest(msg *nats.Msg) {
	subject := msg.Header.Get(messagingSubjectHeader)
	if subject == "" {
		resp, _ := json.Marshal(&agentapi.HostServicesMessagingResponse{
			Errors: []string{"subject is required"},
		})

		err := msg.Respond(resp)
		if err != nil {
			m.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	resp, err := m.nc.Request(subject, msg.Data, messagingRequestTimeout)
	if err != nil {
		m.log.Debug(fmt.Sprintf("failed to send %d-byte request on subject %s: %s", len(msg.Data), subject, err.Error()))

		resp, _ := json.Marshal(&agentapi.HostServicesMessagingResponse{
			Errors: []string{fmt.Sprintf("failed to send %d-byte request on subject %s: %s", len(msg.Data), subject, err.Error())},
		})

		err := msg.Respond(resp)
		if err != nil {
			m.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	m.log.Debug(fmt.Sprintf("received %d-byte response to request on subject: %s", len(resp.Data), subject))

	err = msg.Respond(resp.Data)
	if err != nil {
		m.log.Warn(fmt.Sprintf("failed to respond to messaging host service request: %s", err.Error()))
	}
}

func (m *MessagingService) handleRequestMany(msg *nats.Msg) {
	subject := msg.Header.Get(messagingSubjectHeader)
	if subject == "" {
		resp, _ := json.Marshal(&agentapi.HostServicesMessagingResponse{
			Errors: []string{"subject is required"},
		})

		err := msg.Respond(resp)
		if err != nil {
			m.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	// create a new response inbox and synchronous subscription
	replyTo := m.nc.NewRespInbox()
	sub, err := m.nc.SubscribeSync(replyTo)
	if err != nil {
		resp, _ := json.Marshal(&agentapi.HostServicesMessagingResponse{
			Errors: []string{fmt.Sprintf("failed to subscribe to response inbox: %s", err.Error())},
		})

		err := msg.Respond(resp)
		if err != nil {
			m.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	defer func() {
		_ = sub.Unsubscribe()
	}()

	_ = m.nc.Flush()

	// publish the original requestMany request to the target subject
	err = m.nc.PublishRequest(subject, replyTo, msg.Data)
	if err != nil {
		resp, _ := json.Marshal(&agentapi.HostServicesMessagingResponse{
			Errors: []string{fmt.Sprintf("failed to send %d-byte request to subject: %s: %s", len(msg.Data), subject, err.Error())},
		})

		err := msg.Respond(resp)
		if err != nil {
			m.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}

		return
	}

	start := time.Now()
	for time.Since(start) < messagingRequestManyTimeout {
		resp, err := sub.NextMsg(messagingRequestTimeout)
		if err != nil && !errors.Is(err, nats.ErrTimeout) {
			break
		}

		if resp != nil {
			m.log.Debug(fmt.Sprintf("received %d-byte response to request on subject: %s", len(resp.Data), subject))

			// respond to the requestMany message
			err = msg.Respond(resp.Data)
			if err != nil {
				m.log.Error(fmt.Sprintf("failed to publish %d-byte response to reply subject: %s: %s", len(resp.Data), msg.Reply, err.Error()))
			}
		}
	}
}
