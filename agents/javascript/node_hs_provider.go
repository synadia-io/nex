package agent

import (
	"errors"
	"fmt"

	"github.com/dop251/goja"
)

// TODO: implement this using the builtins client for host services
type NodeHostServicesProvider struct {
	workloadId string
	allocator  vmAllocator
}

var _ hostServiceProvider = &NodeHostServicesProvider{}

func NewNodeHostServicesProvider(workloadId string, allocator vmAllocator) *NodeHostServicesProvider {
	return &NodeHostServicesProvider{
		workloadId: workloadId,
		allocator:  allocator,
	}
}

func (m *NodeHostServicesProvider) KvGet(bucket string, key string) (goja.ArrayBuffer, error) {
	if key == "FAIL" {
		return goja.ArrayBuffer{}, errors.New("FAILY FAIL")
	}

	result := fmt.Sprintf("%s.%s", bucket, key)
	bytes := []byte(result)
	return m.allocator.AllocateByteArray(m.workloadId, bytes), nil
}

func (m *NodeHostServicesProvider) KvSet(bucket string, key string, value goja.ArrayBuffer) error {
	return nil
}
func (m *NodeHostServicesProvider) KvDelete(bucket string, key string) error {
	return nil
}

func (m *NodeHostServicesProvider) KvKeys(bucket string) ([]string, error) {
	return nil, nil
}

func (m *NodeHostServicesProvider) MessagingPublish(subject string, payload goja.ArrayBuffer) error {
	return nil
}

func (m *NodeHostServicesProvider) MessagingRequest(subject string, payload goja.ArrayBuffer) (goja.ArrayBuffer, error) {
	return goja.ArrayBuffer{}, nil
}

func (m *NodeHostServicesProvider) ObjectGet(bucket string, objectName string) (goja.ArrayBuffer, error) {
	return goja.ArrayBuffer{}, nil
}

func (m *NodeHostServicesProvider) ObjectPut(bucket string, objectName string, payload goja.ArrayBuffer) error {
	return nil
}
