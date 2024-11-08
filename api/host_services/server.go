package hostservices

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/api/agent/go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type HostServicesServer struct {
	log        *slog.Logger
	ncInternal *nats.Conn
	services   map[string]HostService
	// Every single workload gets its own private host services connection,
	// even if it's reusing defaults for config
	hsClientConnections map[string]*nats.Conn

	tracer trace.Tracer
}

func NewHostServicesServer(ncInternal *nats.Conn, log *slog.Logger, tracer trace.Tracer) *HostServicesServer {
	return &HostServicesServer{
		log:                 log,
		ncInternal:          ncInternal,
		services:            make(map[string]HostService),
		hsClientConnections: make(map[string]*nats.Conn),
		tracer:              tracer,
	}
}

// Sets the connection used by host services implementations, e.g. key value, object store, etc
func (h *HostServicesServer) SetHostServicesConnection(workloadId string, nc *nats.Conn) {
	h.RemoveHostServicesConnection(workloadId)
	h.hsClientConnections[workloadId] = nc

	// NOTE: there used to be multiple connections per workload Id, but we only support using
	// a single host services connection now for simplicity, this means the account in which
	// trigger subjects are subscribed to is this same host services connection
}

func (h *HostServicesServer) RemoveHostServicesConnection(workloadId string) {
	delete(h.hsClientConnections, workloadId)
}

func (h *HostServicesServer) Services() []string {
	result := make([]string, 0)
	for k := range h.services {
		result = append(result, k)
	}

	return result
}

func (h *HostServicesServer) AddService(name string, svc HostService, config json.RawMessage) error {
	err := svc.Initialize(config)
	if err != nil {
		return err
	}
	h.services[name] = svc

	return nil
}

// Host services server instances subscribe to the following `host.>` subjects,
// which are exported by the `nexnode` account on the configured internal
// NATS connection for consumption by agents:
//
// - host.<workload_type>.rpc.<namespace>.<workloadId>.<service>.<method>
func (h *HostServicesServer) Start() error {
	_, err := h.ncInternal.Subscribe(agentapi.PerformRPCSubscribeSubject(), h.handleRPC)
	if err != nil {
		h.log.Warn("Failed to create Host services rpc subscription", slog.String("error", err.Error()))
		return err
	}

	h.log.Debug("Host services rpc subscription created", slog.String("address", h.ncInternal.ConnectedAddr()))
	return nil
}

func (h *HostServicesServer) handleRPC(msg *nats.Msg) {
	// host.amazeballs.rpc.testspace.abc12346.kv.set
	// workloadType, namespace, workloadId, service, method
	//  0     1              3        4                    5        6
	// host.javascript.rpc.default.couhd3752omu7o74h4fg.messaging.publish

	tokens := strings.Split(msg.Subject, ".")
	namespace := tokens[3]
	workloadID := tokens[4]
	serviceName := tokens[5]
	method := tokens[6]

	h.log.Debug("Handling host service RPC request",
		slog.String("workload_id", workloadID),
		slog.String("service_name", serviceName),
		slog.String("method", method),
	)

	service, ok := h.services[serviceName]
	if !ok {
		serverMsg := serverFailMessage(msg.Reply, 404, fmt.Sprintf("No such host service: %s (subject '%s')", serviceName, msg.Subject))
		_ = msg.RespondMsg(serverMsg)
		return
	}

	metadata := make(map[string]string, 0)
	for k, v := range msg.Header {
		metadata[k] = v[0]
	}

	ctx := otel.GetTextMapPropagator().Extract(context.Background(), propagation.HeaderCarrier(msg.Header))

	_, span := h.tracer.Start(ctx, "host services call",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("workload_id", workloadID),
			attribute.String("service", serviceName),
			attribute.String("method", method),
		))
	defer span.End()

	span.AddEvent("RPC Request Began")

	conn, ok := h.hsClientConnections[workloadID]
	if !ok {
		serverMsg := serverFailMessage(msg.Reply, 500, fmt.Sprintf("No connection found for workload '%s'", workloadID))
		_ = msg.RespondMsg(serverMsg)
		return
	}

	result, err := service.HandleRequest(conn,
		namespace,
		workloadID,
		method,
		metadata,
		msg.Data)
	if err != nil {
		h.log.Warn("Failed to handle host service RPC request",
			slog.String("workload_id", workloadID),
			slog.String("service_name", serviceName),
			slog.String("method", method),
			slog.String("error", err.Error()),
		)
		span.RecordError(err)

		serverMsg := serverFailMessage(msg.Reply, 500, fmt.Sprintf("Failed to execute host service method: %s", err.Error()))
		_ = msg.RespondMsg(serverMsg)
		return
	}

	span.AddEvent("RPC Request Completed")
	var serverMsg *nats.Msg
	if result.IsError() {
		serverMsg = serverFailMessage(msg.Reply, result.Code, result.Message)
	} else {
		serverMsg = serverSuccessMessage(msg.Reply, result.Code, result.Data, messageOk)
	}

	_ = msg.RespondMsg(serverMsg)
}

func serverFailMessage(reply string, code uint, message string) *nats.Msg {
	msg := nats.NewMsg(reply)
	msg.Header.Set(headerCode, fmt.Sprintf("%d", code))
	msg.Header.Set(headerMessage, message)

	return msg
}

func serverSuccessMessage(reply string, code uint, data []byte, message string) *nats.Msg {
	msg := nats.NewMsg(reply)
	msg.Header.Set(headerCode, fmt.Sprintf("%d", code))
	msg.Header.Set(headerMessage, message)
	msg.Data = data

	return msg
}
