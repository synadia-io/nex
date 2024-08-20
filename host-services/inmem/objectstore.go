package inmem

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"

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
)

type InmemObjectStore struct {
	log     *slog.Logger
	objects map[string][]byte
}

func NewInmemObjectStore(log *slog.Logger) *InmemObjectStore {
	return &InmemObjectStore{
		objects: make(map[string][]byte),
		log:     log,
	}
}

func (o *InmemObjectStore) Initialize(_ json.RawMessage) error {
	return nil
}

func (o *InmemObjectStore) HandleRequest(
	conns map[string]*nats.Conn,
	namespace string,
	workloadId string,
	method string,
	workloadName string,
	metadata map[string]string,
	request []byte) (hostservices.ServiceResult, error) {

	switch method {
	case objectStoreServiceMethodGet:
		return o.handleGet(metadata)
	case objectStoreServiceMethodPut:
		return o.handlePut(request, metadata)
	case objectStoreServiceMethodDelete:
		return o.handleDelete(metadata)
	case objectStoreServiceMethodList:
		return o.handleList()
	default:
		o.log.Warn("Received invalid host services RPC request",
			slog.String("service", "objectstore"),
			slog.String("method", method),
		)
		return hostservices.ServiceResultFail(400, "unknown method"), nil
	}
}

func (o *InmemObjectStore) handleGet(metadata map[string]string) (hostservices.ServiceResult, error) {
	name := metadata[agentapi.ObjectStoreObjectNameHeader]
	if name == "" {
		return hostservices.ServiceResultFail(400, "name is required"), nil
	}

	if object, ok := o.objects[name]; ok {
		return hostservices.ServiceResultPass(200, "", object), nil
	}

	return hostservices.ServiceResultFail(404, "no such object"), nil
}

func (o *InmemObjectStore) handlePut(request []byte, metadata map[string]string) (hostservices.ServiceResult, error) {
	name := metadata[agentapi.ObjectStoreObjectNameHeader]
	if name == "" {
		return hostservices.ServiceResultFail(400, "name is required"), nil
	}

	o.objects[name] = request

	res := jetstream.ObjectInfo{
		ObjectMeta: jetstream.ObjectMeta{
			Name:        name,
			Description: "In-memory object",
		},
		Bucket: "bucket",
		Size:   uint64(len(request)),
		Digest: makeDigest(request),
	}
	output, _ := json.Marshal(res)

	return hostservices.ServiceResultPass(200, "", output), nil
}

func (o *InmemObjectStore) handleDelete(
	metadata map[string]string,
) (hostservices.ServiceResult, error) {

	name := metadata[agentapi.ObjectStoreObjectNameHeader]
	if name == "" {
		return hostservices.ServiceResultFail(400, "name is required"), nil
	}

	delete(o.objects, name)

	resp, _ := json.Marshal(&agentapi.HostServicesObjectStoreResponse{
		Success: true,
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (o *InmemObjectStore) handleList() (hostservices.ServiceResult, error) {
	output := make([]jetstream.ObjectInfo, 0)

	for k, v := range o.objects {
		output = append(output, jetstream.ObjectInfo{
			ObjectMeta: jetstream.ObjectMeta{
				Name:        k,
				Description: "In-memory object",
			},
			Bucket: "bucket",
			Size:   uint64(len(v)),
			Digest: makeDigest(v),
		})
	}
	resp, _ := json.Marshal(output)
	return hostservices.ServiceResultPass(200, "", resp), nil
}

// NOTE : digest comes back from NATS as a sha-256 hash base64-encoded in the form SHA-256={hash}
func makeDigest(input []byte) string {
	s := sha256.New()
	s.Write(input)
	hs := s.Sum(nil)

	return fmt.Sprintf("SHA-256=%s", base64.URLEncoding.EncodeToString(hs))
}
