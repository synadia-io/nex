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
	"github.com/synadia-io/nex/models"
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

// StoreWorkload writes the record under compare-and-swap. See
// models.NexNodeState for the expectedRevision contract.
//
// JetStream KV provides both halves natively: Create publishes with an
// expected-last-subject-sequence of 0 (so it fails if the key is occupied),
// and Update publishes with the caller's revision. Both surface a lost race
// as a wrong-last-sequence API error, which is mapped to
// models.ErrStateConflict here so no caller has to reach into jetstream's
// error taxonomy to tell a conflict from a genuine failure.
func (n *natsKVState) StoreWorkload(workloadId string, swr models.StartWorkloadRequest, expectedRevision uint64) error {
	n.Lock()
	defer n.Unlock()

	swrB, err := json.Marshal(swr)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s_%s", swr.WorkloadType, workloadId)
	if expectedRevision == 0 {
		_, err = n.kv.Create(n.ctx, key, swrB)
	} else {
		_, err = n.kv.Update(n.ctx, key, swrB, expectedRevision)
	}
	if err != nil {
		if isRevisionConflict(err) {
			return fmt.Errorf("%w: %s (key %s, expected revision %d)", models.ErrStateConflict, err.Error(), key, expectedRevision)
		}
		return err
	}

	return nil
}

// isRevisionConflict reports whether err is JetStream's "you wrote against a
// revision that is no longer current" answer.
//
// Two shapes reach here for the same underlying condition, so both are
// checked. Create wraps jetstream.ErrKeyExists (itself an APIError carrying
// ErrorCode 10071, stream wrong-last-sequence); Update returns the raw
// *jetstream.APIError with that code. Matching on the error code rather than
// on message text keeps this from silently degrading into "no conflict is
// ever detected", which would behave exactly like the blind Put this
// replaced.
func isRevisionConflict(err error) bool {
	if errors.Is(err, jetstream.ErrKeyExists) {
		return true
	}

	var apiErr *jetstream.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence {
		return true
	}

	return false
}

// GetWorkloadRecord returns the stored definition and the revision to write
// back against. A missing key is the documented (nil, 0, nil) not-found
// signal rather than an error -- see models.NexNodeState.
func (n *natsKVState) GetWorkloadRecord(workloadType, workloadID string) (*models.StartWorkloadRequest, uint64, error) {
	key := fmt.Sprintf("%s_%s", workloadType, workloadID)

	entry, err := n.kv.Get(n.ctx, key)
	if err != nil {
		// A purged (UNDEPLOY) or deleted key reads back as not-found too,
		// which is the right answer: the record is gone, and a caller must
		// not resurrect it.
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil, 0, nil
		}
		return nil, 0, err
	}

	var swr models.StartWorkloadRequest
	if err := json.Unmarshal(entry.Value(), &swr); err != nil {
		return nil, 0, err
	}

	return &swr, entry.Revision(), nil
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
