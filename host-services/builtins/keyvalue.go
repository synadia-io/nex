package builtins

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	hostservices "github.com/synadia-io/nex/host-services"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const kvServiceMethodGet = "get"
const kvServiceMethodSet = "set"
const kvServiceMethodDelete = "delete"
const kvServiceMethodKeys = "keys"

type KeyValueService struct {
	log *slog.Logger
	nc  *nats.Conn
}

func NewKeyValueService(nc *nats.Conn, log *slog.Logger) (*KeyValueService, error) {
	kv := &KeyValueService{
		log: log,
		nc:  nc,
	}

	return kv, nil
}

func (k *KeyValueService) Initialize(_ map[string]string) error {
	return nil
}

func (k *KeyValueService) HandleRequest(
	namespace string,
	workloadId string,
	method string,
	workloadName string,
	metadata map[string]string,
	request []byte) (hostservices.ServiceResult, error) {

	switch method {
	case kvServiceMethodGet:
		return k.handleGet(workloadId, workloadName, request, metadata, namespace)
	case kvServiceMethodSet:
		return k.handleSet(workloadId, workloadName, request, metadata, namespace)
	case kvServiceMethodDelete:
		return k.handleDelete(workloadId, workloadName, request, metadata, namespace)
	case kvServiceMethodKeys:
		return k.handleKeys(workloadId, workloadName, request, metadata, namespace)
	default:
		k.log.Warn("Received invalid host services RPC request",
			slog.String("service", "kv"),
			slog.String("method", method),
		)
		return hostservices.ServiceResultFail(400, "Received invalid host services RPC request"), nil
	}
}

func (k *KeyValueService) handleGet(
	_, workload string,
	_ []byte, metadata map[string]string,
	namespace string,
) (hostservices.ServiceResult, error) {

	kvStore, err := k.resolveKeyValueStore(namespace, workload)
	if err != nil {
		k.log.Error(fmt.Sprintf("failed to resolve key/value store: %s", err.Error()))
		return hostservices.ServiceResultFail(500, "could not resolve k/v store"), nil
	}

	key := metadata[agentapi.KeyValueKeyHeader]
	if key == "" {
		return hostservices.ServiceResultFail(400, "key is required"), nil
	}

	entry, err := kvStore.Get(key)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to get value for key %s: %s", key, err.Error()))
		return hostservices.ServiceResultFail(500, "failed to get value for key"), nil
	}

	return hostservices.ServiceResultPass(200, "", entry.Value()), nil
}

func (k *KeyValueService) handleSet(_, workload string,
	data []byte, metadata map[string]string,
	namespace string) (hostservices.ServiceResult, error) {

	kvStore, err := k.resolveKeyValueStore(namespace, workload)
	if err != nil {
		k.log.Error(fmt.Sprintf("failed to resolve key/value store: %s", err.Error()))
		return hostservices.ServiceResultFail(500, "could not resolve k/v store"), nil
	}

	key := metadata[agentapi.KeyValueKeyHeader]
	if key == "" {
		return hostservices.ServiceResultFail(400, "key is required"), nil
	}

	revision, err := kvStore.Put(key, data)
	if err != nil {
		k.log.Error(fmt.Sprintf("failed to write %d-byte value for key %s: %s", len(data), key, err.Error()))
		return hostservices.ServiceResultFail(500, "failed to write value for key"), nil
	}

	resp, _ := json.Marshal(map[string]interface{}{
		"revision": revision,
		"success":  true,
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (k *KeyValueService) handleDelete(_, workload string,
	_ []byte, metadata map[string]string,
	namespace string) (hostservices.ServiceResult, error) {

	kvStore, err := k.resolveKeyValueStore(namespace, workload)
	if err != nil {
		k.log.Error(fmt.Sprintf("failed to resolve key/value store: %s", err.Error()))
		return hostservices.ServiceResultFail(500, "could not resolve k/v store"), nil
	}

	key := metadata[agentapi.KeyValueKeyHeader]
	if key == "" {
		return hostservices.ServiceResultFail(400, "key is required"), nil
	}

	err = kvStore.Delete(key)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to delete key %s: %s", key, err.Error()))
		return hostservices.ServiceResultFail(500, "failed to delete key"), nil
	}

	resp, _ := json.Marshal(map[string]interface{}{
		"success": true,
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (k *KeyValueService) handleKeys(_, workload string,
	_ []byte, _ map[string]string,
	namespace string) (hostservices.ServiceResult, error) {

	kvStore, err := k.resolveKeyValueStore(namespace, workload)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to resolve key/value store: %s", err.Error()))
		return hostservices.ServiceResultFail(500, "could not resolve k/v store"), nil
	}

	// TODO: deprecated, switch to a KeysLister channel
	keys, err := kvStore.Keys() // TODO-- paginate...
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to respond to key/value host service request: %s", err.Error()))
		return hostservices.ServiceResultFail(500, "failed to resolve keys"), nil
	}

	resp, _ := json.Marshal(keys)

	return hostservices.ServiceResultPass(200, "", resp), nil
}

// resolve the key value store for this workload; initialize it if necessary
func (k *KeyValueService) resolveKeyValueStore(namespace, workload string) (nats.KeyValue, error) {
	js, err := k.nc.JetStream()
	if err != nil {
		return nil, err
	}

	kvStoreName := fmt.Sprintf("hs_%s_%s_kv", namespace, workload)
	kvStore, err := js.KeyValue(kvStoreName)
	if err != nil {
		if errors.Is(err, nats.ErrBucketNotFound) {
			// TODO: make this configurable after kubecon
			kvStore, err = js.CreateKeyValue(&nats.KeyValueConfig{Bucket: kvStoreName, MaxBytes: 524288})
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return kvStore, nil
}
