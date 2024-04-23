package lib

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const objectStoreServiceMethodGet = "get"
const objectStoreServiceMethodPut = "put"
const objectStoreServiceMethodDelete = "delete"
const objectStoreServiceMethodList = "list"

const objectStoreObjectNameHeader = "x-object-name"

type ObjectStoreService struct {
	log *slog.Logger
	nc  *nats.Conn
}

func NewObjectStoreService(nc *nats.Conn, log *slog.Logger) (*ObjectStoreService, error) {
	objectStore := &ObjectStoreService{
		log: log,
		nc:  nc,
	}

	err := objectStore.init()
	if err != nil {
		return nil, err
	}

	return objectStore, nil
}

func (o *ObjectStoreService) init() error {
	return nil
}

func (o *ObjectStoreService) HandleRPC(msg *nats.Msg) {
	// agentint.{vmID}.rpc.{namespace}.{workload}.{service}.{method}
	tokens := strings.Split(msg.Subject, ".")
	service := tokens[5]
	method := tokens[6]

	switch method {
	case objectStoreServiceMethodGet:
		o.handleGet(msg)
	case objectStoreServiceMethodPut:
		o.handlePut(msg)
	case objectStoreServiceMethodDelete:
		o.handleDelete(msg)
	case objectStoreServiceMethodList:
		o.handleList(msg)
	default:
		o.log.Warn("Received invalid host services RPC request",
			slog.String("service", service),
			slog.String("method", method),
		)

		// msg.Respond()
	}
}

func (o *ObjectStoreService) handleGet(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	namespace := tokens[3]
	workload := tokens[4]

	objectStore, err := o.resolveObjectStore(namespace, workload)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to resolve object store: %s", err.Error()))
	}

	name := msg.Header.Get(objectStoreObjectNameHeader)
	if name == "" {
		resp, _ := json.Marshal(&agentapi.HostServicesObjectStoreResponse{
			Errors: []string{"name is required"},
		})

		err := msg.Respond(resp)
		if err != nil {
			o.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	result, err := objectStore.Get(name)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to get object %s: %s", name, err.Error()))

		resp, _ := json.Marshal(&agentapi.HostServicesObjectStoreResponse{
			Errors: []string{fmt.Sprintf("failed to get object %s: %s", name, err.Error())},
		})

		err := msg.Respond(resp)
		if err != nil {
			o.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	val, err := io.ReadAll(result)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to get object %s: %s", name, err.Error()))

		resp, _ := json.Marshal(&agentapi.HostServicesObjectStoreResponse{
			Errors: []string{fmt.Sprintf("failed to get object %s: %s", name, err.Error())},
		})

		err := msg.Respond(resp)
		if err != nil {
			o.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	err = msg.Respond(val)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to respond to object store host service request: %s", err.Error()))
	}
}

func (o *ObjectStoreService) handlePut(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	namespace := tokens[3]
	workload := tokens[4]

	objectStore, err := o.resolveObjectStore(namespace, workload)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to resolve object store: %s", err.Error()))
	}

	name := msg.Header.Get(objectStoreObjectNameHeader)
	if name == "" {
		resp, _ := json.Marshal(&agentapi.HostServicesObjectStoreResponse{
			Errors: []string{"name is required"},
		})

		err := msg.Respond(resp)
		if err != nil {
			o.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	result, err := objectStore.Put(&nats.ObjectMeta{
		Name: name,
		// TODO Description
		// TODO Headers
		// TODO Metadata
		// TODO Opts
	}, bufio.NewReader(bytes.NewReader(msg.Data)))
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to write %d-byte object %s: %s", len(msg.Data), name, err.Error()))

		resp, _ := json.Marshal(&agentapi.HostServicesObjectStoreResponse{
			Errors:  []string{fmt.Sprintf("failed to write %d-byte object %s: %s", len(msg.Data), name, err.Error())},
			Success: false,
		})

		err := msg.Respond(resp)
		if err != nil {
			o.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	resp, _ := json.Marshal(result)
	err = msg.Respond(resp)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to respond to object store host service request: %s", err.Error()))
	}
}

func (o *ObjectStoreService) handleDelete(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	namespace := tokens[3]
	workload := tokens[4]

	objectStore, err := o.resolveObjectStore(namespace, workload)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to resolve object store: %s", err.Error()))
	}

	name := msg.Header.Get(objectStoreObjectNameHeader)
	if name == "" {
		resp, _ := json.Marshal(&agentapi.HostServicesObjectStoreResponse{
			Errors: []string{"name is required"},
		})

		err := msg.Respond(resp)
		if err != nil {
			o.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	err = objectStore.Delete(name)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to delete object %s: %s", name, err.Error()))

		resp, _ := json.Marshal(&agentapi.HostServicesObjectStoreResponse{
			Errors: []string{fmt.Sprintf("failed to delete object %s: %s", name, err.Error())},
		})

		err := msg.Respond(resp)
		if err != nil {
			o.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	resp, _ := json.Marshal(&agentapi.HostServicesObjectStoreResponse{
		Success: true,
	})
	err = msg.Respond(resp)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to respond to object store host service request: %s", err.Error()))
	}
}

func (o *ObjectStoreService) handleList(msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	namespace := tokens[3]
	workload := tokens[4]

	objectStore, err := o.resolveObjectStore(namespace, workload)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to resolve object store: %s", err.Error()))
	}

	objects, err := objectStore.List() // TODO-- paginate...
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to respond to object store host service request: %s", err.Error()))

		resp, _ := json.Marshal(&agentapi.HostServicesObjectStoreResponse{
			Errors: []string{fmt.Sprintf("failed to list objects: %s", err.Error())},
		})

		err := msg.Respond(resp)
		if err != nil {
			o.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}

		return
	}

	resp, _ := json.Marshal(objects)
	err = msg.Respond(resp)
	if err != nil {
		o.log.Warn(fmt.Sprintf("failed to respond to object store host service request: %s", err.Error()))
	}
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
