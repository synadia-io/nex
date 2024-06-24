package builtins

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

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
	BucketName   string `json:"bucket_name"`
	MaxBytes     int    `json:"max_bytes"`
	JitProvision bool   `json:"jit_provision"`
}

func NewKeyValueService(log *slog.Logger) (*KeyValueService, error) {
	kv := &KeyValueService{
		log: log,
	}

	return kv, nil
}

func (k *KeyValueService) Initialize(config json.RawMessage) error {

	k.config.BucketName = "hs_${namespace}_${workload_name}_kv"
	k.config.JitProvision = true
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
	conns []*nats.Conn,
	namespace string,
	workloadId string,
	method string,
	workloadName string,
	metadata map[string]string,
	request []byte,
) (hostservices.ServiceResult, error) {
	nc := conns[0]
	switch method {
	case kvServiceMethodGet:
		return k.handleGet(nc, workloadId, workloadName, request, metadata, namespace)
	case kvServiceMethodSet:
		return k.handleSet(nc, workloadId, workloadName, request, metadata, namespace)
	case kvServiceMethodDelete:
		return k.handleDelete(nc, workloadId, workloadName, request, metadata, namespace)
	case kvServiceMethodKeys:
		return k.handleKeys(nc, workloadId, workloadName, request, metadata, namespace)
	default:
		k.log.Warn("Received invalid host services RPC request",
			slog.String("service", "kv"),
			slog.String("method", method),
		)
		return hostservices.ServiceResultFail(400, "Received invalid host services RPC request"), nil
	}
}

func (k *KeyValueService) handleGet(
	nc *nats.Conn,
	_, workload string,
	_ []byte, metadata map[string]string,
	namespace string,
) (hostservices.ServiceResult, error) {

	kvStore, err := k.resolveKeyValueStore(nc, namespace, workload)
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

func (k *KeyValueService) handleSet(
	nc *nats.Conn,
	_, workload string,
	data []byte, metadata map[string]string,
	namespace string) (hostservices.ServiceResult, error) {

	kvStore, err := k.resolveKeyValueStore(nc, namespace, workload)
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

func (k *KeyValueService) handleDelete(
	nc *nats.Conn,
	_, workload string,
	_ []byte, metadata map[string]string,
	namespace string) (hostservices.ServiceResult, error) {

	kvStore, err := k.resolveKeyValueStore(nc, namespace, workload)
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

func (k *KeyValueService) handleKeys(
	nc *nats.Conn,
	_, workload string,
	_ []byte, _ map[string]string,
	namespace string) (hostservices.ServiceResult, error) {

	kvStore, err := k.resolveKeyValueStore(nc, namespace, workload)
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
func (k *KeyValueService) resolveKeyValueStore(nc *nats.Conn, namespace, workload string) (nats.KeyValue, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}

	reWorkload := regexp.MustCompile(`(?i)\$\{workload_name\}`)
	reNamespace := regexp.MustCompile(`(?i)\$\{namespace\}`)

	kvStoreName := reWorkload.ReplaceAllString(k.config.BucketName, workload)
	kvStoreName = reNamespace.ReplaceAllString(kvStoreName, namespace)

	kvStore, err := js.KeyValue(kvStoreName)
	if err != nil {
		if errors.Is(err, nats.ErrBucketNotFound) && k.config.JitProvision {
			kvStore, err = js.CreateKeyValue(&nats.KeyValueConfig{Bucket: kvStoreName, MaxBytes: int64(k.config.MaxBytes)})
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	k.log.Debug("Resolved key/value store for KV host service", slog.String("name", kvStoreName), slog.String("bucket", kvStore.Bucket()))
	return kvStore, nil
}
