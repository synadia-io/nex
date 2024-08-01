package builtins

import (
	"encoding/json"
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
	log    *slog.Logger
	config kvConfig
}

type kvConfig struct {
	BucketName string `json:"bucket_name"`
	MaxBytes   int    `json:"max_bytes"`
}

func NewKeyValueService(log *slog.Logger) (*KeyValueService, error) {
	kv := &KeyValueService{
		log: log,
	}

	return kv, nil
}

func (k *KeyValueService) Initialize(config json.RawMessage) error {

	k.config.MaxBytes = 524288

	if len(config) > 0 {
		err := json.Unmarshal(config, &k.config)
		if err != nil {
			return err
		}
	}

	return nil
}

func (k *KeyValueService) HandleRequest(
	conns map[string]*nats.Conn,
	namespace string,
	workloadId string,
	method string,
	workloadName string,
	metadata map[string]string,
	request []byte,
) (hostservices.ServiceResult, error) {
	var nc *nats.Conn

	if conns[hostservices.HostServicesConnection] != nil {
		nc = conns[hostservices.HostServicesConnection]
	} else {
		nc = conns[hostservices.DefaultConnection]
	}

	kv, err := k.resolveKeyValueStore(nc, metadata)
	if err != nil {
		k.log.Error("Failed to locate host services KV bucket", slog.String("bucket", metadata[agentapi.BucketContextHeader]))
		return hostservices.ServiceResultFail(400, "Could not find host services KV bucket"), nil
	}

	switch method {
	case kvServiceMethodGet:
		return k.handleGet(kv, metadata)
	case kvServiceMethodSet:
		return k.handleSet(kv, request, metadata)
	case kvServiceMethodDelete:
		return k.handleDelete(kv, metadata)
	case kvServiceMethodKeys:
		return k.handleKeys(kv)
	default:
		k.log.Warn("Received invalid host services RPC request",
			slog.String("service", "kv"),
			slog.String("method", method),
		)
		return hostservices.ServiceResultFail(400, "Received invalid host services RPC request"), nil
	}
}

func (k *KeyValueService) handleGet(
	kvStore nats.KeyValue,
	metadata map[string]string,
) (hostservices.ServiceResult, error) {

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

func (k *KeyValueService) handleSet(
	kvStore nats.KeyValue,
	data []byte,
	metadata map[string]string,
) (hostservices.ServiceResult, error) {

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

func (k *KeyValueService) handleDelete(
	kvStore nats.KeyValue,
	metadata map[string]string,
) (hostservices.ServiceResult, error) {

	key := metadata[agentapi.KeyValueKeyHeader]
	if key == "" {
		return hostservices.ServiceResultFail(400, "key is required"), nil
	}

	err := kvStore.Delete(key)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to delete key %s: %s", key, err.Error()))
		return hostservices.ServiceResultFail(500, "failed to delete key"), nil
	}

	resp, _ := json.Marshal(map[string]interface{}{
		"success": true,
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (k *KeyValueService) handleKeys(kvStore nats.KeyValue) (hostservices.ServiceResult, error) {

	keyLister, err := kvStore.ListKeys()
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to respond to key/value host service request: %s", err.Error()))
		return hostservices.ServiceResultFail(500, "failed to resolve keys"), nil
	}
	keys := make([]string, 0)
	for key := range keyLister.Keys() {
		keys = append(keys, key)
	}

	resp, _ := json.Marshal(keys)

	return hostservices.ServiceResultPass(200, "", resp), nil
}

// resolve the key value store for this workload; initialize it if necessary
func (k *KeyValueService) resolveKeyValueStore(nc *nats.Conn, metadata map[string]string) (nats.KeyValue, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	kvStoreName := metadata[agentapi.BucketContextHeader]

	kvStore, err := js.KeyValue(kvStoreName)
	if err != nil {
		return nil, err
	}

	k.log.Debug("Resolved key/value store for KV host service", slog.String("name", kvStoreName), slog.String("bucket", kvStore.Bucket()))
	return kvStore, nil
}
