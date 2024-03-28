package nexnode

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	controlapi "github.com/synadia-io/nex/internal/control-api"
)

func (m *MachineManager) CacheWorkload(request *controlapi.DeployRequest) (uint64, *string, error) {
	bucket := request.Location.Host
	key := strings.Trim(request.Location.Path, "/")

	m.log.Info("Attempting object store download", slog.String("bucket", bucket), slog.String("key", key), slog.String("url", m.nc.Opts.Url))

	opts := []nats.JSOpt{}
	if request.JsDomain != nil {
		opts = append(opts, nats.APIPrefix(*request.JsDomain))
	}

	js, err := m.nc.JetStream(opts...)
	if err != nil {
		return 0, nil, err
	}

	store, err := js.ObjectStore(bucket)
	if err != nil {
		m.log.Error("Failed to bind to source object store", slog.Any("err", err), slog.String("bucket", bucket))
		return 0, nil, err
	}

	_, err = store.GetInfo(key)
	if err != nil {
		m.log.Error("Failed to locate workload binary in source object store", slog.Any("err", err), slog.String("key", key), slog.String("bucket", bucket))
		return 0, nil, err
	}

	workload, err := store.GetBytes(key)
	if err != nil {
		m.log.Error("Failed to download bytes from source object store", slog.Any("err", err), slog.String("key", key))
		return 0, nil, err
	}

	jsInternal, err := m.ncInternal.JetStream()
	if err != nil {
		m.log.Error("Failed to acquire JetStream context for internal object store.", slog.Any("err", err))
		panic(err)
	}

	cache, err := jsInternal.ObjectStore(agentapi.WorkloadCacheBucket)
	if err != nil {
		m.log.Error("Failed to get object store reference for internal cache.", slog.Any("err", err))
		panic(err)
	}

	obj, err := cache.PutBytes(request.DecodedClaims.Subject, workload)
	if err != nil {
		m.log.Error("Failed to write workload to internal cache.", slog.Any("err", err))
		panic(err)
	}

	workloadHash := sha256.New()
	workloadHash.Write(workload)
	workloadHashString := hex.EncodeToString(workloadHash.Sum(nil))

	m.log.Info("Successfully stored workload in internal object store", slog.String("name", request.DecodedClaims.Subject), slog.Int64("bytes", int64(obj.Size)))
	return obj.Size, &workloadHashString, nil
}
