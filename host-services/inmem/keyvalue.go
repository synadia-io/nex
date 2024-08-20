package inmem

import (
	"encoding/json"
	"log/slog"

	"github.com/nats-io/nats.go"
	hostservices "github.com/synadia-io/nex/host-services"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const kvServiceMethodGet = "get"
const kvServiceMethodSet = "set"
const kvServiceMethodDelete = "delete"
const kvServiceMethodKeys = "keys"

type InmemKeyValue struct {
	log     *slog.Logger
	kvstore map[string][]byte
}

func NewInmemKeyValueService(log *slog.Logger) *InmemKeyValue {
	return &InmemKeyValue{
		log:     log,
		kvstore: make(map[string][]byte),
	}
}

func (k *InmemKeyValue) Initialize(_ json.RawMessage) error {
	return nil
}

func (k *InmemKeyValue) HandleRequest(
	conns map[string]*nats.Conn,
	namespace string,
	workloadId string,
	method string,
	workloadName string,
	metadata map[string]string,
	request []byte,
) (hostservices.ServiceResult, error) {

	switch method {
	case kvServiceMethodGet:
		return k.handleGet(metadata)
	case kvServiceMethodSet:
		return k.handleSet(request, metadata)
	case kvServiceMethodDelete:
		return k.handleDelete(metadata)
	case kvServiceMethodKeys:
		return k.handleKeys()
	default:
		k.log.Warn("Received invalid host services RPC request",
			slog.String("service", "kv"),
			slog.String("method", method),
		)
		return hostservices.ServiceResultFail(400, "Received invalid host services RPC request"), nil
	}
}

func (k *InmemKeyValue) handleGet(
	metadata map[string]string,
) (hostservices.ServiceResult, error) {

	key := metadata[agentapi.KeyValueKeyHeader]
	if key == "" {
		return hostservices.ServiceResultFail(400, "key is required"), nil
	}
	if val, ok := k.kvstore[key]; ok {
		return hostservices.ServiceResultPass(200, "", val), nil
	}
	return hostservices.ServiceResultFail(404, "no such key"), nil
}

func (k *InmemKeyValue) handleSet(
	data []byte,
	metadata map[string]string,
) (hostservices.ServiceResult, error) {

	key := metadata[agentapi.KeyValueKeyHeader]
	if key == "" {
		return hostservices.ServiceResultFail(400, "key is required"), nil
	}
	k.kvstore[key] = data
	resp, _ := json.Marshal(map[string]interface{}{
		"revision": 1,
		"success":  true,
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (k *InmemKeyValue) handleDelete(
	metadata map[string]string,
) (hostservices.ServiceResult, error) {

	key := metadata[agentapi.KeyValueKeyHeader]
	if key == "" {
		return hostservices.ServiceResultFail(400, "key is required"), nil
	}

	delete(k.kvstore, key)

	resp, _ := json.Marshal(map[string]interface{}{
		"success": true,
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (k *InmemKeyValue) handleKeys() (hostservices.ServiceResult, error) {
	keys := make([]string, 0)
	for key := range k.kvstore {
		keys = append(keys, key)
	}

	resp, _ := json.Marshal(keys)

	return hostservices.ServiceResultPass(200, "", resp), nil
}
