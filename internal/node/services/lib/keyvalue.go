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
		k.handleKeyValueGet(msg)
	case kvServiceMethodSet:
		k.handleKeyValueSet(msg)
	case kvServiceMethodDelete:
		k.handleKeyValueDelete(msg)
	case kvServiceMethodKeys:
		k.handleKeyValueKeys(msg)
	default:
		k.log.Warn("Received invalid host services RPC request",
			slog.String("service", service),
			slog.String("method", method),
		)

		// msg.Respond()
	}
}

func (k *KeyValueService) handleKeyValueGet(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	namespace := tokens[3]
	workload := tokens[4]

	kvStore, err := k.resolveKeyValueStore(namespace, workload)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to resolve key/value store: %s", err.Error()))
	}

	var req *agentapi.HostServicesKeyValueRequest
	err = json.Unmarshal(msg.Data, &req)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to unmarshal key/value RPC request: %s", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to unmarshal key/value RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	if req.Key == nil {
		resp, _ := json.Marshal(map[string]interface{}{
			"error": "key is required",
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	entry, err := kvStore.Get(*req.Key)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to get value for key %s: %s", *req.Key, err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to delete keys %s: %s", *req.Key, err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	val := json.RawMessage(entry.Value())
	resp, _ := json.Marshal(&agentapi.HostServicesKeyValueRequest{
		Key:   req.Key,
		Value: &val,
	})
	err = msg.Respond(resp)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to respond to key/value host service request: %s", err.Error()))
	}
}

func (k *KeyValueService) handleKeyValueSet(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	namespace := tokens[3]
	workload := tokens[4]

	kvStore, err := k.resolveKeyValueStore(namespace, workload)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to resolve key/value store: %s", err.Error()))
	}

	var req *agentapi.HostServicesKeyValueRequest
	err = json.Unmarshal(msg.Data, &req)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to unmarshal key/value RPC request: %s", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to unmarshal key/value RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	// FIXME-- add req.Validate()
	if req.Key == nil {
		resp, _ := json.Marshal(map[string]interface{}{
			"error": "key is required",
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	// FIXME-- add req.Validate()
	if req.Value == nil {
		resp, _ := json.Marshal(map[string]interface{}{
			"error": "value is required",
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	revision, err := kvStore.Put(*req.Key, *req.Value)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to write %d-byte value for key %s: %s", len(*req.Value), *req.Key, err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to write %d-byte value for key %s: %s", len(*req.Value), *req.Key, err.Error()),
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

func (k *KeyValueService) handleKeyValueDelete(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	namespace := tokens[3]
	workload := tokens[4]

	kvStore, err := k.resolveKeyValueStore(namespace, workload)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to resolve key/value store: %s", err.Error()))
	}

	var req *agentapi.HostServicesKeyValueRequest
	err = json.Unmarshal(msg.Data, &req)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to unmarshal key/value RPC request: %s", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to unmarshal key/value RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	if req.Key == nil {
		resp, _ := json.Marshal(map[string]interface{}{
			"error": "key is required",
		})

		err := msg.Respond(resp)
		if err != nil {
			k.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	err = kvStore.Delete(*req.Key)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to delete key %s: %s", *req.Key, err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to delete keys %s: %s", *req.Key, err.Error()),
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

func (k *KeyValueService) handleKeyValueKeys(msg *nats.Msg) {
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
			kvStore, err = js.CreateKeyValue(&nats.KeyValueConfig{Bucket: kvStoreName})
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return kvStore, nil
}
