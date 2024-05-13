package builtins

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	hostservices "github.com/synadia-io/nex/host-services"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const objectStoreServiceMethodGet = "get"
const objectStoreServiceMethodPut = "put"
const objectStoreServiceMethodDelete = "delete"
const objectStoreServiceMethodList = "list"

type ObjectStoreService struct {
	log *slog.Logger
	nc  *nats.Conn
}

func NewObjectStoreService(nc *nats.Conn, log *slog.Logger) (*ObjectStoreService, error) {
	objectStore := &ObjectStoreService{
		log: log,
		nc:  nc,
	}

	return objectStore, nil
}

func (o *ObjectStoreService) Initialize(_ map[string]string) error {
	return nil
}

func (o *ObjectStoreService) HandleRequest(
	namespace string,
	workloadId string,
	method string,
	workloadName string,
	metadata map[string]string,
	request []byte) (hostservices.ServiceResult, error) {

	switch method {
	case objectStoreServiceMethodGet:
		return o.handleGet(workloadId, workloadName, request, metadata, namespace)
	case objectStoreServiceMethodPut:
		return o.handlePut(workloadId, workloadName, request, metadata, namespace)
	case objectStoreServiceMethodDelete:
		return o.handleDelete(workloadId, workloadName, request, metadata, namespace)
	case objectStoreServiceMethodList:
		return o.handleList(workloadId, workloadName, request, metadata, namespace)
	default:
		o.log.Warn("Received invalid host services RPC request",
			slog.String("service", "objectstore"),
			slog.String("method", method),
		)
		return hostservices.ServiceResultFail(400, "unknown method"), nil
	}
}

func (o *ObjectStoreService) handleGet(_, workload string,
	_ []byte, metadata map[string]string,
	namespace string,
) (hostservices.ServiceResult, error) {

	objectStore, err := o.resolveObjectStore(namespace, workload)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to resolve object store: %s", err.Error()))
		return hostservices.ServiceResultFail(500, "unable to resolve object store"), nil
	}

	name := metadata[agentapi.ObjectStoreObjectNameHeader]
	if name == "" {
		return hostservices.ServiceResultFail(400, "name is required"), nil
	}

	result, err := objectStore.Get(name)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to get object %s: %s", name, err.Error()))
		code := uint(500)
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			code = 404
		}
		return hostservices.ServiceResultFail(code, "failed to get object"), nil
	}

	val, err := io.ReadAll(result)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to get object %s: %s", name, err.Error()))
		return hostservices.ServiceResultFail(500, "failed to read object data"), nil
	}

	return hostservices.ServiceResultPass(200, "", val), nil
}

func (o *ObjectStoreService) handlePut(_, workload string,
	data []byte, metadata map[string]string,
	namespace string,
) (hostservices.ServiceResult, error) {

	objectStore, err := o.resolveObjectStore(namespace, workload)
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

func (o *ObjectStoreService) handleDelete(_, workload string,
	_ []byte, metadata map[string]string,
	namespace string,
) (hostservices.ServiceResult, error) {

	objectStore, err := o.resolveObjectStore(namespace, workload)
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

func (o *ObjectStoreService) handleList(_, workload string,
	_ []byte, _ map[string]string,
	namespace string,
) (hostservices.ServiceResult, error) {

	objectStore, err := o.resolveObjectStore(namespace, workload)
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

// resolve the object store for the given workload; initialize it if necessary
func (o *ObjectStoreService) resolveObjectStore(namespace, workload string) (nats.ObjectStore, error) {
	js, err := o.nc.JetStream()
	if err != nil {
		return nil, err
	}

	objectStoreName := fmt.Sprintf("hs_%s_%s_os", namespace, workload)
	objectStore, err := js.ObjectStore(objectStoreName)
	if err != nil {
		if errors.Is(err, nats.ErrStreamNotFound) {
			objectStore, err = js.CreateObjectStore(&nats.ObjectStoreConfig{Bucket: objectStoreName, MaxBytes: 524288}) // FIXME-- make configurable
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return objectStore, nil
}
