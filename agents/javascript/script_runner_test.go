package agent

import (
	"errors"
	"fmt"
	"testing"

	"github.com/dop251/goja"
	"github.com/nats-io/nuid"
)

func setupSuite(t testing.TB) (*ScriptRunner, string, func(tb testing.TB)) {
	// I love how this would make the Rust compiler send a hit squad after me
	runner := NewScriptRunner(testProviderFactory)

	return runner, nuid.Next(), func(tb testing.TB) {
		// do cleanup
	}
}

func TestRawObject(t *testing.T) {
	runner, workloadId, teardown := setupSuite(t)
	defer teardown(t)

	err := runner.AddScript(workloadId,
		`
		function run() {
		return { name: "John", age: 30 };
	}

		`)
	if err != nil {
		t.Error(err)
		return
	}

	res, err := runner.TriggerScript(workloadId, []byte{})
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("%s", string(res))
}

func TestKvGet(t *testing.T) {
	runner, workloadId, teardown := setupSuite(t)
	defer teardown(t)

	err := runner.AddScript(workloadId,
		`
	function run() {
		return host.kvGet("bucket", "key");
        }
`)
	if err != nil {
		t.Error(err)
		return
	}

	res, err := runner.TriggerScript(workloadId, []byte{})
	fmt.Printf("%#v\n", res)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println(string(res))
}

func TestKvGetError(t *testing.T) {
	runner, workloadId, teardown := setupSuite(t)
	defer teardown(t)

	err := runner.AddScript(workloadId,
		`
	function run() {
		return host.kvGet("bucket", "FAIL");
        }
`)
	if err != nil {
		t.Error(err)
		return
	}

	_, err = runner.TriggerScript(workloadId, []byte{})
	if err == nil {
		t.Error(errors.New("Should have returned an error but didn't"))
		return
	}
	fmt.Printf("%+v\n", err)

}

type mockHost struct {
	workloadId string
	allocator  vmAllocator
}

var _ hostServiceProvider = &mockHost{}

func testProviderFactory(workloadId string, allocator vmAllocator) hostServiceProvider {
	return &mockHost{
		workloadId: workloadId,
		allocator:  allocator,
	}
}

func (m *mockHost) KvGet(bucket string, key string) (goja.ArrayBuffer, error) {
	if key == "FAIL" {
		return goja.ArrayBuffer{}, errors.New("FAILY FAIL")
	}

	result := fmt.Sprintf("%s.%s", bucket, key)
	bytes := []byte(result)
	return m.allocator.AllocateByteArray(m.workloadId, bytes), nil
}

func (m *mockHost) KvSet(bucket string, key string, value goja.ArrayBuffer) error {
	return nil
}
func (m *mockHost) KvDelete(bucket string, key string) error {
	return nil
}

func (m *mockHost) KvKeys(bucket string) ([]string, error) {
	return nil, nil
}

func (m *mockHost) MessagingPublish(subject string, payload goja.ArrayBuffer) error {
	return nil
}

func (m *mockHost) MessagingRequest(subject string, payload goja.ArrayBuffer) (goja.ArrayBuffer, error) {
	return goja.ArrayBuffer{}, nil
}

func (m *mockHost) ObjectGet(bucket string, objectName string) (goja.ArrayBuffer, error) {
	return goja.ArrayBuffer{}, nil
}

func (m *mockHost) ObjectPut(bucket string, objectName string, payload goja.ArrayBuffer) error {
	return nil
}
