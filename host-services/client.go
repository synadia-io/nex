package hostservices

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type HostServicesClient struct {
	nc           *nats.Conn
	namespace    string
	workloadName string
	workloadId   string
	timeout      time.Duration
}

func NewHostServicesClient(nc *nats.Conn, timeout time.Duration, namespace, workloadName, workloadId string) *HostServicesClient {
	return &HostServicesClient{
		nc:           nc,
		namespace:    namespace,
		workloadName: workloadName,
		workloadId:   workloadId,
		timeout:      timeout,
	}
}

func (c *HostServicesClient) PerformRPC(ctx context.Context, service string, method string, payload []byte, metadata map[string]string) (ServiceResult, error) {
	subject := fmt.Sprintf("hostint.%s.rpc.%s.%s.%s.%s",
		c.workloadId,
		c.namespace,
		c.workloadName,
		service,
		method,
	)

	msg := nats.NewMsg(subject)
	msg.Data = payload

	for k, v := range metadata {
		msg.Header.Set(k, v)
	}

	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(msg.Header))

	result, err := c.nc.RequestMsg(msg, c.timeout)
	if err != nil {
		return ServiceResult{}, err
	}
	code := result.Header.Get(headerCode)
	iCode, _ := strconv.Atoi(code)

	message := result.Header.Get(headerMessage)
	serviceResult := ServiceResult{
		Data:    result.Data,
		Message: message,
		Code:    uint(iCode),
	}

	return serviceResult, nil
}
