package builtins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	hostservices "github.com/synadia-io/nex/node/services"
)

const KeyValueKeyHeader = "x-keyvalue-key"

const kvServiceMethodGet = "get"
const kvServiceMethodSet = "set"
const kvServiceMethodDelete = "delete"
const kvServiceMethodKeys = "keys"

const kvTimeout = 1500 * time.Millisecond

const defaultKeyValueBucketName = "HostServices_KeyValue"

type KeyValueService struct {
	log    *slog.Logger
	config kvConfig
}

type kvConfig struct {
	MaxBytes      int    `json:"max_bytes"`
	AutoProvision bool   `json:"auto_provision"`
	JsDomain      string `json:"js_domain"`
}

func NewKeyValueService(log *slog.Logger) (*KeyValueService, error) {
	kv := &KeyValueService{
		log: log,
	}

	return kv, nil
}

func (k *KeyValueService) Initialize(config json.RawMessage) error {

	k.config.MaxBytes = 524288
	k.config.AutoProvision = true

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
		k.log.Error("Failed to resolve host services KV bucket", slog.String("bucket", metadata[BucketContextHeader]))
		code := uint(500)
		if errors.Is(err, jetstream.ErrBucketNotFound) {
			code = 404
		}
		return hostservices.ServiceResultFail(code, "Could not resolve host services KV bucket"), nil
	}
	ctx, cancelF := context.WithTimeout(context.Background(), kvTimeout)
	defer cancelF()

	switch method {
	case kvServiceMethodGet:
		return k.handleGet(ctx, kv, metadata)
	case kvServiceMethodSet:
		return k.handleSet(ctx, kv, request, metadata)
	case kvServiceMethodDelete:
		return k.handleDelete(ctx, kv, metadata)
	case kvServiceMethodKeys:
		return k.handleKeys(ctx, kv)
	default:
		k.log.Warn("Received invalid host services RPC request",
			slog.String("service", "kv"),
			slog.String("method", method),
		)
		return hostservices.ServiceResultFail(400, "Received invalid host services RPC request"), nil
	}
}

func (k *KeyValueService) handleGet(
	ctx context.Context,
	kvStore jetstream.KeyValue,
	metadata map[string]string,
) (hostservices.ServiceResult, error) {

	key := metadata[KeyValueKeyHeader]
	if key == "" {
		return hostservices.ServiceResultFail(400, "key is required"), nil
	}

	entry, err := kvStore.Get(ctx, key)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to get value for key %s: %s", key, err.Error()))
		return hostservices.ServiceResultFail(500, "failed to get value for key"), nil
	}

	return hostservices.ServiceResultPass(200, "", entry.Value()), nil
}

func (k *KeyValueService) handleSet(
	ctx context.Context,
	kvStore jetstream.KeyValue,
	data []byte,
	metadata map[string]string,
) (hostservices.ServiceResult, error) {

	key := metadata[KeyValueKeyHeader]
	if key == "" {
		return hostservices.ServiceResultFail(400, "key is required"), nil
	}

	revision, err := kvStore.Put(ctx, key, data)
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
	ctx context.Context,
	kvStore jetstream.KeyValue,
	metadata map[string]string,
) (hostservices.ServiceResult, error) {

	key := metadata[KeyValueKeyHeader]
	if key == "" {
		return hostservices.ServiceResultFail(400, "key is required"), nil
	}

	err := kvStore.Delete(ctx, key)
	if err != nil {
		k.log.Warn(fmt.Sprintf("failed to delete key %s: %s", key, err.Error()))
		return hostservices.ServiceResultFail(500, "failed to delete key"), nil
	}

	resp, _ := json.Marshal(map[string]interface{}{
		"success": true,
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (k *KeyValueService) handleKeys(ctx context.Context, kvStore jetstream.KeyValue) (hostservices.ServiceResult, error) {

	keyLister, err := kvStore.ListKeys(ctx)
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
func (k *KeyValueService) resolveKeyValueStore(nc *nats.Conn, metadata map[string]string) (jetstream.KeyValue, error) {
	var js jetstream.JetStream
	var err error

	ctx := context.Background()

	if len(k.config.JsDomain) > 0 {
		js, err = jetstream.NewWithDomain(nc, k.config.JsDomain)
	} else {
		js, err = jetstream.New(nc)
	}
	if err != nil {
		return nil, err
	}
	kvStoreName := metadata[BucketContextHeader]

	kvStore, err := js.KeyValue(ctx, kvStoreName)
	if err != nil && errors.Is(err, jetstream.ErrBucketNotFound) && k.config.AutoProvision {
		if len(kvStoreName) == 0 {
			kvStoreName = defaultKeyValueBucketName
		}
		kvStore, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:   kvStoreName,
			MaxBytes: int64(k.config.MaxBytes),
		})
		if err != nil {
			return nil, err
		}
		return kvStore, nil
	} else if err != nil {
		return nil, err
	}

	k.log.Debug("Resolved key/value store for KV host service", slog.String("name", kvStoreName), slog.String("bucket", kvStore.Bucket()))
	return kvStore, nil
}
