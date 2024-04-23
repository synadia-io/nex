package lib

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const kvServiceMethodGet = "get"
const kvServiceMethodSet = "set"
const kvServiceMethodDelete = "delete"
const kvServiceMethodKeys = "keys"

const keyValueKeyHeader = "x-key"

type KeyValueService struct {
	log *slog.Logger
	nc  *nats.Conn
}

func NewKeyValueService(nc *nats.Conn, log *slog.Logger) (*KeyValueService, error) {
	kv := &KeyValueService{
		log: log,
		nc:  nc,
	}

	err := kv.init()
	if err != nil {
		return nil, err
	}

	return kv, nil
}

func (k *KeyValueService) init() error {
	return nil
}

func (k *KeyValueService) HandleRPC(msg *nats.Msg) {
	// agentint.{vmID}.rpc.{namespace}.{workload}.{service}.{method}
	tokens := strings.Split(msg.Subject, ".")
	service := tokens[5]
	method := tokens[6]

	switch method {
	case kvServiceMethodGet:
		k.handleGet(msg)
	case kvServiceMethodSet:
		k.handleSet(msg)
	case kvServiceMethodDelete:
		k.handleDelete(msg)
	case kvServiceMethodKeys:
		k.handleKeys(msg)
	default:
		k.log.Warn("Received invalid host services RPC request",
			slog.String("service", service),
			slog.String("method", method),
		)

		// msg.Respond()
	}
}

func (k *KeyValueService) handleGet(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	namespace := tokens[3]
	workload := tokens[4]

	kvStore, err := k.resolveKeyValueStore(namespace, workload)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to resolve key/value store: %s", err.Error()))
	}

	key := msg.Header.Get(keyValueKeyHeader)
	if key == "" {
		resp, _ := json.Marshal(&agentapi.HostServicesKeyValueResponse{
			Errors: []string{"key is required"},
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	entry, err := kvStore.Get(key)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to get value for key %s: %s", key, err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to get value for key %s: %s", key, err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	err = msg.Respond(entry.Value())
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to respond to key/value host service request: %s", err.Error()))
	}
}

func (k *KeyValueService) handleSet(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	namespace := tokens[3]
	workload := tokens[4]

	kvStore, err := k.resolveKeyValueStore(namespace, workload)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to resolve key/value store: %s", err.Error()))
	}

	key := msg.Header.Get(keyValueKeyHeader)
	if key == "" {
		resp, _ := json.Marshal(&agentapi.HostServicesKeyValueResponse{
			Errors: []string{"key is required"},
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	revision, err := kvStore.Put(key, msg.Data)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to write %d-byte value for key %s: %s", len(msg.Data), key, err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to write %d-byte value for key %s: %s", len(msg.Data), key, err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	resp, _ := json.Marshal(map[string]interface{}{
		"revision": revision,
		"success":  true,
	})
	err = msg.Respond(resp)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to respond to key/value host service request: %s", err.Error()))
	}
}

func (k *KeyValueService) handleDelete(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	namespace := tokens[3]
	workload := tokens[4]

	kvStore, err := k.resolveKeyValueStore(namespace, workload)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to resolve key/value store: %s", err.Error()))
	}

	key := msg.Header.Get(keyValueKeyHeader)
	if key == "" {
		resp, _ := json.Marshal(&agentapi.HostServicesKeyValueResponse{
			Errors: []string{"key is required"},
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	err = kvStore.Delete(key)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to delete key %s: %s", key, err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to delete key %s: %s", key, err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	resp, _ := json.Marshal(map[string]interface{}{
		"success": true,
	})
	err = msg.Respond(resp)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to respond to key/value host service request: %s", err.Error()))
	}
}

func (k *KeyValueService) handleKeys(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	namespace := tokens[3]
	workload := tokens[4]

	kvStore, err := k.resolveKeyValueStore(namespace, workload)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to resolve key/value store: %s", err.Error()))
	}

	keys, err := kvStore.Keys() // TODO-- paginate...
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to respond to key/value host service request: %s", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to resolve keys: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}

		return
	}

	resp, _ := json.Marshal(keys)
	err = msg.Respond(resp)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to respond to key/value host service request: %s", err.Error()))
	}
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
