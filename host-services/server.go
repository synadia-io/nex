package hostservices

import (
	"fmt"
	"strings"

	"github.com/nats-io/nats.go"
)

type HostServicesServer struct {
	nc       *nats.Conn
	services map[string]HostService
}

func NewHostServicesServer(nc *nats.Conn) *HostServicesServer {
	return &HostServicesServer{
		nc:       nc,
		services: make(map[string]HostService),
	}
}

func (h *HostServicesServer) AddService(name string, svc HostService, config map[string]string) error {
	err := svc.Initialize(config)
	if err != nil {
		return err
	}
	h.services[name] = svc

	return nil
}

func (h *HostServicesServer) Start() error {
	_, err := h.nc.Subscribe("agentint.*.rpc.*.*.*.*", h.handleRPC)
	if err != nil {
		return err
	}

	return nil
}

func (h *HostServicesServer) handleRPC(msg *nats.Msg) {
	// agentint.{vmID}.rpc.{namespace}.{workload}.{service}.{method}
	tokens := strings.Split(msg.Subject, ".")
	vmID := tokens[1]
	namespace := tokens[3]
	workloadName := tokens[4]
	serviceName := tokens[5]
	method := tokens[6]

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
