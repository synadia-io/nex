package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/synadia-labs/nex/models"
)

var _ models.NexNodeState = (*natsKVState)(nil)

type natsKVState struct {
	sync.Mutex

	ctx    context.Context
	logger *slog.Logger
	kv     jetstream.KeyValue
}

func NewNatsKVState(nc *nats.Conn, bucketName string, logger *slog.Logger) (*natsKVState, error) {
	ctx := context.Background()
	ret := &natsKVState{
		ctx:    ctx,
		logger: logger,
	}

	jsCtx, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}

	ret.kv, err = jsCtx.CreateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket:       bucketName,
		MaxBytes:     100_000_000, // 100MB
		MaxValueSize: 10_000,      // 10KB
	})
	if err != nil && !errors.Is(err, jetstream.ErrBucketExists) {
		return nil, err
	}
	if errors.Is(err, jetstream.ErrBucketExists) {
		ret.kv, err = jsCtx.KeyValue(context.Background(), bucketName)
		if err != nil {
			return nil, err
		}
	}

	return ret, nil
}

func (n *natsKVState) StoreWorkload(workloadId string, swr models.StartWorkloadRequest) error {
	n.Lock()
	defer n.Unlock()

	swrB, err := json.Marshal(swr)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s_%s", swr.WorkloadType, workloadId)
	_, err = n.kv.Put(n.ctx, key, swrB)
	if err != nil {
		return err
	}

	return nil
}

func (n *natsKVState) RemoveWorkload(workloadType, workloadId string) error {
	n.Lock()
	defer n.Unlock()

	key := fmt.Sprintf("%s_%s", workloadType, workloadId)
	return n.kv.Purge(n.ctx, key)
}

// GetStateForAgent returns the state of the NexNode for a given agent in
// the form of a map of workloadId to StartWorkloadRequest
func (n *natsKVState) GetStateByAgent(agentName string) (map[string]models.StartWorkloadRequest, error) {
	kl, err := n.kv.ListKeys(n.ctx)
	if err != nil {
		return nil, err
	}

	ret := make(map[string]models.StartWorkloadRequest)
	for k := range kl.Keys() {
		if strings.HasPrefix(k, agentName) {
			v, err := n.kv.Get(n.ctx, k)
			if err != nil {
				return nil, err
			}

			var swr models.StartWorkloadRequest
			err = json.Unmarshal(v.Value(), &swr)
			if err != nil {
				return nil, err
			}

			ret[strings.TrimPrefix(k, fmt.Sprintf("%s_", agentName))] = swr
		}
	}
	return ret, nil
}

func (n *natsKVState) GetStateByNamespace(namespace string) (map[string]models.StartWorkloadRequest, error) {
	kl, err := n.kv.ListKeys(n.ctx)
	if err != nil {
		return nil, err
	}

	ret := make(map[string]models.StartWorkloadRequest)
	for k := range kl.Keys() {
		v, err := n.kv.Get(n.ctx, k)
		if err != nil {
			return nil, err
		}

		var swr models.StartWorkloadRequest
		err = json.Unmarshal(v.Value(), &swr)
		if err != nil {
			return nil, err
		}

		if swr.Namespace == namespace {
			workloadId := strings.Split(k, "_")
			if len(workloadId) != 2 {
				n.logger.Warn("invalid workloadId", slog.String("key", k))
				continue
			}
			ret[workloadId[1]] = swr
		}
	}
	return ret, nil
}
