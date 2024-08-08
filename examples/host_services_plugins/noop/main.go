package main

import (
	"encoding/json"

	"github.com/nats-io/nats.go"
	hostservices "github.com/synadia-io/nex/host-services"
)

type noopService struct{}

func (n *noopService) Initialize(config json.RawMessage) error {
	return nil
}

func (n *noopService) HandleRequest(
	connections map[string]*nats.Conn,
	namespace string,
	workloadId string,
	method string,
	workloadName string,
	metadata map[string]string,
	request []byte,
) (hostservices.ServiceResult, error) {
	return hostservices.ServiceResultPass(200, "OK", []byte{}), nil
}

// exported
var HostServiceProvider noopService
