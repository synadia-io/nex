package builtins

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	hostservices "github.com/synadia-io/nex/host-services"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const (
	objectStoreServiceMethodGet    = "get"
	objectStoreServiceMethodPut    = "put"
	objectStoreServiceMethodDelete = "delete"
	objectStoreServiceMethodList   = "list"

	defaultMaxBytes   = 524288
	defaultBucketName = "hs_${namespace}_${workload_name}_obj"
)

type ObjectStoreService struct {
	log    *slog.Logger
	config objectStoreConfig
}

type objectStoreConfig struct {
	BucketName   string `json:"bucket_name"`
	MaxBytes     int    `json:"max_bytes"`
	JitProvision bool   `json:"jit_provision"`
}

func NewObjectStoreService(log *slog.Logger) (*ObjectStoreService, error) {
	objectStore := &ObjectStoreService{
		log: log,
	}

	return objectStore, nil
}

func (o *ObjectStoreService) Initialize(config json.RawMessage) error {

	o.config.BucketName = defaultBucketName
	o.config.JitProvision = true
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
	// TODO:(jr) might need to rename "TriggerConnection" to upstream or something similiar as its starting to be reused
	if conns[hostservices.TriggerConnection] != nil {
		nc = conns[hostservices.TriggerConnection]
	} else {
		nc = conns[hostservices.DefaultConnection]
	}

	switch method {
	case objectStoreServiceMethodGet:
		return o.handleGet(nc, workloadId, workloadName, request, metadata, namespace)
	case objectStoreServiceMethodPut:
		return o.handlePut(nc, workloadId, workloadName, request, metadata, namespace)
	case objectStoreServiceMethodDelete:
		return o.handleDelete(nc, workloadId, workloadName, request, metadata, namespace)
	case objectStoreServiceMethodList:
		return o.handleList(nc, workloadId, workloadName, request, metadata, namespace)
	default:
		o.log.Warn("Received invalid host services RPC request",
			slog.String("service", "objectstore"),
			slog.String("method", method),
		)
		return hostservices.ServiceResultFail(400, "unknown method"), nil
	}
}

func (o *ObjectStoreService) handleGet(
	nc *nats.Conn,
	_, workload string,
	_ []byte, metadata map[string]string,
	namespace string,
) (hostservices.ServiceResult, error) {

	objectStore, err := o.resolveObjectStore(nc, namespace, workload)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to resolve object store: %s", err.Error()))
		return hostservices.ServiceResultFail(500, "unable to resolve object store"), nil
	}

	name := metadata[agentapi.ObjectStoreObjectNameHeader]
	if name == "" {
		return hostservices.ServiceResultFail(400, "name is required"), nil
	}

	start := time.Now()
	result, err := objectStore.Get(name)
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
	nc *nats.Conn,
	_, workload string,
	data []byte, metadata map[string]string,
	namespace string,
) (hostservices.ServiceResult, error) {

	objectStore, err := o.resolveObjectStore(nc, namespace, workload)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to resolve object store: %s", err.Error()))
		return hostservices.ServiceResultFail(500, "failed to resolve object store"), nil
	}

	name := metadata[agentapi.ObjectStoreObjectNameHeader]
	if name == "" {
		return hostservices.ServiceResultFail(400, "name is required"), nil
	}

	result, err := objectStore.Put(&nats.ObjectMeta{
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
	nc *nats.Conn,
	_, workload string,
	_ []byte, metadata map[string]string,
	namespace string,
) (hostservices.ServiceResult, error) {

	objectStore, err := o.resolveObjectStore(nc, namespace, workload)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to resolve object store: %s", err.Error()))
		return hostservices.ServiceResultFail(500, "failed to resolve object store"), nil
	}

	name := metadata[agentapi.ObjectStoreObjectNameHeader]
	if name == "" {
		return hostservices.ServiceResultFail(400, "name is required"), nil
	}

	err = objectStore.Delete(name)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to delete object %s: %s", name, err.Error()))
		return hostservices.ServiceResultFail(500, "failed to delete object"), nil
	}

	resp, _ := json.Marshal(&agentapi.HostServicesObjectStoreResponse{
		Success: true,
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (o *ObjectStoreService) handleList(
	nc *nats.Conn,
	_, workload string,
	_ []byte, _ map[string]string,
	namespace string,
) (hostservices.ServiceResult, error) {

	objectStore, err := o.resolveObjectStore(nc, namespace, workload)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to resolve object store: %s", err.Error()))
		return hostservices.ServiceResultFail(500, "failed to resolve object store"), nil
	}

	objects, err := objectStore.List() // TODO-- paginate...
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to respond to object store host service request: %s", err.Error()))
		return hostservices.ServiceResultFail(500, "failed to list objects"), nil
	}

	resp, _ := json.Marshal(objects)
	return hostservices.ServiceResultPass(200, "", resp), nil
}

// resolve the object store for the given workload; initialize it if necessary & configured to do so
func (o *ObjectStoreService) resolveObjectStore(nc *nats.Conn, namespace, workload string) (nats.ObjectStore, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}

	reWorkload := regexp.MustCompile(`(?i)\$\{workload_name\}`)
	reNamespace := regexp.MustCompile(`(?i)\$\{namespace\}`)

	objectStoreName := reWorkload.ReplaceAllString(o.config.BucketName, workload)
	objectStoreName = reNamespace.ReplaceAllString(objectStoreName, namespace)

	objectStore, err := js.ObjectStore(objectStoreName)
	if err != nil {
		if errors.Is(err, nats.ErrStreamNotFound) && o.config.JitProvision {
			objectStore, err = js.CreateObjectStore(&nats.ObjectStoreConfig{Bucket: objectStoreName, MaxBytes: int64(o.config.MaxBytes)})
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return objectStore, nil
}
