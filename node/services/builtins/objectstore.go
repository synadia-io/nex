package builtins

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	hostservices "github.com/synadia-io/nex/node/services"
)

const (
	ObjectStoreObjectNameHeader = "x-object-name"

	objectStoreServiceMethodGet    = "get"
	objectStoreServiceMethodPut    = "put"
	objectStoreServiceMethodDelete = "delete"
	objectStoreServiceMethodList   = "list"

	defaultMaxBytes         = 524288
	objectTimeout           = 1500 * time.Millisecond
	defaultObjectBucketName = "HostServices_ObjectStore"
)

type ObjectStoreService struct {
	log    *slog.Logger
	config objectStoreConfig
}

type objectStoreConfig struct {
	MaxBytes      int    `json:"max_bytes"`
	AutoProvision bool   `json:"auto_provision"`
	JsDomain      string `json:"js_domain"`
}

func NewObjectStoreService(log *slog.Logger) (*ObjectStoreService, error) {
	objectStore := &ObjectStoreService{
		log: log,
	}

	return objectStore, nil
}

func (o *ObjectStoreService) Initialize(config json.RawMessage) error {

	o.config.AutoProvision = true
	o.config.MaxBytes = defaultMaxBytes

	if len(config) > 0 {
		err := json.Unmarshal(config, &o.config)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *ObjectStoreService) HandleRequest(
	conns map[string]*nats.Conn,
	namespace string,
	workloadId string,
	method string,
	workloadName string,
	metadata map[string]string,
	request []byte) (hostservices.ServiceResult, error) {

	var nc *nats.Conn
	if conns[hostservices.HostServicesConnection] != nil {
		nc = conns[hostservices.HostServicesConnection]
	} else {
		nc = conns[hostservices.DefaultConnection]
	}
	objectStoreName := metadata[BucketContextHeader]
	ctx, cancelF := context.WithTimeout(context.Background(), kvTimeout)
	defer cancelF()

	objectStore, err := o.resolveObjectStore(ctx, nc, objectStoreName)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to resolve object store: %s", err.Error()))
		code := uint(500)
		if errors.Is(err, jetstream.ErrBucketNotFound) {
			code = 404
		}
		return hostservices.ServiceResultFail(code, "unable to resolve object store"), nil
	}

	switch method {
	case objectStoreServiceMethodGet:
		return o.handleGet(ctx, objectStore, metadata)
	case objectStoreServiceMethodPut:
		return o.handlePut(ctx, objectStore, request, metadata)
	case objectStoreServiceMethodDelete:
		return o.handleDelete(ctx, objectStore, metadata)
	case objectStoreServiceMethodList:
		return o.handleList(ctx, objectStore)
	default:
		o.log.Warn("Received invalid host services RPC request",
			slog.String("service", "objectstore"),
			slog.String("method", method),
		)
		return hostservices.ServiceResultFail(400, "unknown method"), nil
	}
}

// TODO: stop bleeding jetstream ObjectInfo type in the JSON payload
// and use a more abstract, host-services type
func (o *ObjectStoreService) handleGet(
	ctx context.Context,
	objectStore jetstream.ObjectStore,
	metadata map[string]string,
) (hostservices.ServiceResult, error) {

	name := metadata[ObjectStoreObjectNameHeader]
	if name == "" {
		return hostservices.ServiceResultFail(400, "name is required"), nil
	}

	start := time.Now()
	result, err := objectStore.Get(ctx, name)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to get object %s: %s", name, err.Error()))
		code := uint(500)
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			code = 404
		}
		return hostservices.ServiceResultFail(code, "failed to get object"), nil
	}
	finished := time.Since(start)
	o.log.Debug("Object store download complete", slog.String("name", name), slog.String("duration", fmt.Sprintf("%.2f sec", finished.Seconds())))

	val, err := io.ReadAll(result)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to get object %s: %s", name, err.Error()))
		return hostservices.ServiceResultFail(500, "failed to read object data"), nil
	}

	return hostservices.ServiceResultPass(200, "", val), nil
}

func (o *ObjectStoreService) handlePut(
	ctx context.Context,
	objectStore jetstream.ObjectStore,
	data []byte, metadata map[string]string,
) (hostservices.ServiceResult, error) {

	name := metadata[ObjectStoreObjectNameHeader]
	if name == "" {
		return hostservices.ServiceResultFail(400, "name is required"), nil
	}

	result, err := objectStore.Put(ctx, jetstream.ObjectMeta{
		Name: name,
		// TODO Description
		// TODO Headers
		// TODO Metadata
		// TODO Opts
	}, bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to write %d-byte object %s: %s", len(data), name, err.Error()))

		return hostservices.ServiceResultFail(500, ""), nil
	}

	resp, _ := json.Marshal(result)
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (o *ObjectStoreService) handleDelete(
	ctx context.Context,
	objectStore jetstream.ObjectStore,
	metadata map[string]string,
) (hostservices.ServiceResult, error) {

	name := metadata[ObjectStoreObjectNameHeader]
	if name == "" {
		return hostservices.ServiceResultFail(400, "name is required"), nil
	}

	err := objectStore.Delete(ctx, name)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to delete object %s: %s", name, err.Error()))
		return hostservices.ServiceResultFail(500, "failed to delete object"), nil
	}

	resp, _ := json.Marshal(&HostServicesObjectStoreResponse{
		Success: true,
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (o *ObjectStoreService) handleList(
	ctx context.Context,
	objectStore jetstream.ObjectStore,
) (hostservices.ServiceResult, error) {

	objects, err := objectStore.List(ctx)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to respond to object store host service request: %s", err.Error()))
		return hostservices.ServiceResultFail(500, "failed to list objects"), nil
	}

	resp, _ := json.Marshal(objects)
	return hostservices.ServiceResultPass(200, "", resp), nil
}

// resolve the object store for the given workload; initialize it if necessary & configured to do so
func (o *ObjectStoreService) resolveObjectStore(ctx context.Context, nc *nats.Conn, objectStoreName string) (jetstream.ObjectStore, error) {
	var js jetstream.JetStream
	var err error

	if len(o.config.JsDomain) > 0 {
		js, err = jetstream.NewWithDomain(nc, o.config.JsDomain)
	} else {
		js, err = jetstream.New(nc)
	}
	if err != nil {
		return nil, err
	}

	objectStore, err := js.ObjectStore(ctx, objectStoreName)
	if err != nil && errors.Is(err, jetstream.ErrBucketNotFound) && o.config.AutoProvision {
		if len(objectStoreName) == 0 {
			objectStoreName = defaultObjectBucketName
		}
		objectStore, err := js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{
			Bucket:   objectStoreName,
			MaxBytes: int64(o.config.MaxBytes),
		})
		if err != nil {
			return nil, err
		}
		return objectStore, nil
	} else if err != nil {
		return nil, err
	}

	return objectStore, nil
}
