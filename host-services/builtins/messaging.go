package builtins

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	hostservices "github.com/synadia-io/nex/host-services"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const messagingServiceMethodPublish = "publish"
const messagingServiceMethodRequest = "request"
const messagingServiceMethodRequestMany = "requestMany"

const messagingRequestTimeout = time.Millisecond * 500 // FIXME-- make timeout configurable per request?
const messagingRequestManyTimeout = time.Millisecond * 3000

type MessagingService struct {
	log *slog.Logger
	nc  *nats.Conn
}

func NewMessagingService(nc *nats.Conn, log *slog.Logger) (*MessagingService, error) {
	messaging := &MessagingService{
		log: log,
		nc:  nc,
	}

	return messaging, nil
}

func (m *MessagingService) Initialize(_ map[string]string) error {
	return nil
}

func (m *MessagingService) HandleRequest(namespace string,
	workloadId string,
	method string,
	workloadName string,
	metadata map[string]string,
	request []byte) (hostservices.ServiceResult, error) {

	switch method {
	case messagingServiceMethodPublish:
		return m.handlePublish(workloadId, workloadName, request, metadata, namespace)
	case messagingServiceMethodRequest:
		return m.handleRequest(workloadId, workloadName, request, metadata, namespace)
	case messagingServiceMethodRequestMany:
		return m.handleRequestMany(workloadId, workloadName, request, metadata, namespace)
	default:
		m.log.Warn("Received invalid host services RPC request",
			slog.String("service", "messaging"),
			slog.String("method", method),
		)
		return hostservices.ServiceResultFail(400, "unknown method"), nil
	}
}

func (m *MessagingService) handlePublish(_, _ string,
	data []byte, metadata map[string]string,
	_ string,
) (hostservices.ServiceResult, error) {
	subject := metadata[agentapi.MessagingSubjectHeader]
	if subject == "" {
		return hostservices.ServiceResultFail(500, "subject is required"), nil
	}

	err := m.nc.Publish(subject, data)
	if err != nil {
		m.log.Warn(fmt.Sprintf("failed to publish %d-byte message on subject %s: %s", len(data), subject, err.Error()))
		return hostservices.ServiceResultFail(500, "failed to publish message"), nil
	}
	resp, _ := json.Marshal(&agentapi.HostServicesMessagingResponse{
		Success: true,
	})

	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (m *MessagingService) handleRequest(_, _ string,
	data []byte, metadata map[string]string,
	_ string,
) (hostservices.ServiceResult, error) {
	subject := metadata[agentapi.MessagingSubjectHeader]
	if subject == "" {
		return hostservices.ServiceResultFail(400, "subject is required"), nil
	}

	resp, err := m.nc.Request(subject, data, messagingRequestTimeout)
	if err != nil {
		m.log.Debug(fmt.Sprintf("failed to send %d-byte request on subject %s: %s", len(data), subject, err.Error()))
		return hostservices.ServiceResultFail(500, "failed to send request"), nil
	}

	m.log.Debug(fmt.Sprintf("received %d-byte response to request on subject: %s", len(resp.Data), subject))
	return hostservices.ServiceResultPass(200, "", resp.Data), nil
}

func (m *MessagingService) handleRequestMany(_, _ string,
	_ []byte, metadata map[string]string,
	_ string,
) (hostservices.ServiceResult, error) {
	subject := metadata[agentapi.MessagingSubjectHeader]
	if subject == "" {
		return hostservices.ServiceResultFail(400, "subject is required"), nil
	}

	return hostservices.ServiceResultFail(500, "not yet supported"), nil

	// // create a new response inbox and synchronous subscription
	// replyTo := m.nc.NewRespInbox()
	// sub, err := m.nc.SubscribeSync(replyTo)
	// if err != nil {
	// 	return hostservices.ServiceResultFail(500, "failed to subscribe to response inbox"), nil
	// }

	// defer func() {
	// 	_ = sub.Unsubscribe()
	// }()

	// _ = m.nc.Flush()

	// // publish the original requestMany request to the target subject
	// err = m.nc.PublishRequest(subject, replyTo, data)
	// if err != nil {
	// 	return hostservices.ServiceResultFail(500, "failed to send request on subject"), nil
	// }

	// results := make([][]byte, 0)

	// start := time.Now()
	// for time.Since(start) < messagingRequestManyTimeout {
	// 	resp, err := sub.NextMsg(messagingRequestTimeout)
	// 	if err != nil && !errors.Is(err, nats.ErrTimeout) {
	// 		break
	// 	}

	// 	if resp != nil {
	// 		m.log.Debug(fmt.Sprintf("received %d-byte response to request on subject: %s", len(resp.Data), subject))
	// 		results = append(results, resp.Data)
	// 	}
	// }

	// return hostservices.ServiceResultPass(200, "", )
}
