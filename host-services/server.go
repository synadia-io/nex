package hostservices

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
)

type HostServicesServer struct {
	ncInternal *nats.Conn
	services   map[string]HostService

	log *slog.Logger
}

func NewHostServicesServer(nc *nats.Conn, log *slog.Logger) *HostServicesServer {
	return &HostServicesServer{
		ncInternal: nc,
		log:        log,
		services:   make(map[string]HostService),
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

func (h *HostServicesServer) Start() error {
	_, err := h.ncInternal.Subscribe("agentint.*.rpc.*.*.*.*", h.handleRPC)
	if err != nil {
		return err
	}

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
		fmt.Printf("%+v", h.services)
		serverMsg := serverFailMessage(msg.Reply, 404, fmt.Sprintf("No such host service: %s", serviceName))
		_ = msg.RespondMsg(serverMsg)
		return
	}

	metadata := make(map[string]string, 0)
	for k, v := range msg.Header {
		metadata[k] = v[0]
	}

	result, err := service.HandleRequest(namespace, vmID, method, workloadName, metadata, msg.Data)
	if err != nil {
		// TODO: log the err.Error()
		serverMsg := serverFailMessage(msg.Reply, 500, fmt.Sprintf("Failed to execute host service method: %s", err.Error()))
		_ = msg.RespondMsg(serverMsg)
		return
	}

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
