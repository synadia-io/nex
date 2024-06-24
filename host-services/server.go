package hostservices

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
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
	hsClientConnections map[string][]*nats.Conn

	tracer trace.Tracer
}

func NewHostServicesServer(ncInternal *nats.Conn, log *slog.Logger, tracer trace.Tracer) *HostServicesServer {
	return &HostServicesServer{
		log:                 log,
		ncInternal:          ncInternal,
		services:            make(map[string]HostService),
		hsClientConnections: make(map[string][]*nats.Conn),
		tracer:              tracer,
	}
}

// Sets the connection used by host services implementations, e.g. key value, object store, etc
func (h *HostServicesServer) SetHostServicesConnection(workloadId string, nc *nats.Conn) {
	h.RemoveHostServicesConnection(workloadId)
	h.hsClientConnections[workloadId] = []*nats.Conn{nc}
}

// Adds a secondary host services client used by providers like messaging to connect to the same subject
// space as the workload's corresponding triggers
func (h *HostServicesServer) AddHostServicesConnection(workloadId string, nc *nats.Conn) {
	var cs []*nats.Conn
	if conns, ok := h.hsClientConnections[workloadId]; ok {
		cs = append(conns, nc)
	} else {
		cs = []*nats.Conn{nc}
	}

	h.hsClientConnections[workloadId] = cs

}

func (h *HostServicesServer) RemoveHostServicesConnection(workloadId string) {
	if conns, ok := h.hsClientConnections[workloadId]; ok {
		for _, c := range conns {
			_ = c.Drain()
		}
		delete(h.hsClientConnections, workloadId)
	}
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

// Host services server instances subscribe to the following `hostint.>` subjects,
// which are exported by the `nexnode` account on the configured internal
// NATS connection for consumption by agents:
//
// - hostint.<agent_id>.rpc.<namespace>.<workloadName>.<service>.<method>
func (h *HostServicesServer) Start() error {
	_, err := h.ncInternal.Subscribe("hostint.*.rpc.*.*.*.*", h.handleRPC)
	if err != nil {
		h.log.Warn("Failed to create Host services rpc subscription", slog.String("error", err.Error()))
		return err
	}

	h.log.Debug("Host services rpc subscription created", slog.String("address", h.ncInternal.ConnectedAddr()))
	return nil
}

func (h *HostServicesServer) handleRPC(msg *nats.Msg) {
	// agentint.couhd3752omu7o74h4fg.rpc.default.httpjs.http.get
	// agentint.{vmID}.rpc.{namespace}.{workload}.{service}.{method}
	tokens := strings.Split(msg.Subject, ".")
	vmID := tokens[1]
	namespace := tokens[3]
	workloadName := tokens[4]
	serviceName := tokens[5]
	method := tokens[6]

	h.log.Debug("Handling host service RPC request",
		slog.String("workload_id", vmID),
		slog.String("workload_name", workloadName),
		slog.String("service_name", serviceName),
		slog.String("method", method),
	)

	service, ok := h.services[serviceName]
	if !ok {
		serverMsg := serverFailMessage(msg.Reply, 404, fmt.Sprintf("No such host service: %s", serviceName))
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
			attribute.String("workload_id", vmID),
			attribute.String("service", serviceName),
			attribute.String("method", method),
		))
	defer span.End()

	span.AddEvent("RPC Request Began")

	conns := h.hsClientConnections[vmID]

	result, err := service.HandleRequest(conns, namespace, vmID, method, workloadName, metadata, msg.Data)
	if err != nil {
		h.log.Warn("Failed to handle host service RPC request",
			slog.String("workload_id", vmID),
			slog.String("workload_name", workloadName),
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

	serverMsg := serverSuccessMessage(msg.Reply, result.Code, result.Data, messageOk)
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
