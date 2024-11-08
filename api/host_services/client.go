package hostservices

import (
	"context"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/api/agent/go"
	/*
		"go.opentelemetry.io/otel"
		"go.opentelemetry.io/otel/propagation"
	*/)

// The HostServicesClient is to be used by an agent for communicating with host services
// on behalf of a managed workload
type HostServicesClient struct {
	ncInternal   *nats.Conn
	namespace    string
	workloadName string
	workloadId   string
	timeout      time.Duration
}

func NewHostServicesClient(
	ncInternal *nats.Conn,
	timeout time.Duration,
	namespace, workloadName, workloadId string) *HostServicesClient {
	return &HostServicesClient{
		ncInternal:   ncInternal,
		namespace:    namespace,
		workloadName: workloadName,
		workloadId:   workloadId,
		timeout:      timeout,
	}
}

func (c *HostServicesClient) PerformRPC(
	ctx context.Context,
	workloadType string,
	service string,
	method string,
	payload []byte,
	metadata map[string]string) (ServiceResult, error) {
	subject := agentapi.PerformRPCSubject(workloadType,
		c.workloadId,
		c.namespace,
		service,
		method,
	)

	msg := nats.NewMsg(subject)
	msg.Data = payload

	for k, v := range metadata {
		msg.Header.Set(k, v)
	}

	//	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(msg.Header))

	result, err := c.ncInternal.RequestMsg(msg, c.timeout)
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
