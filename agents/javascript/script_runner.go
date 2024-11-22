package agent

import (
	"errors"

	"github.com/dop251/goja"
)

type ScriptRunner struct {
	factory providerFactory
	scripts map[string]*goja.Runtime
}

type hostServiceProvider interface {
	KvGet(bucket string, key string) (goja.ArrayBuffer, error)
	KvSet(bucket string, key string, value goja.ArrayBuffer) error
	KvDelete(bucket string, key string) error
	KvKeys(bucket string) ([]string, error)

	MessagingPublish(subject string, payload goja.ArrayBuffer) error
	MessagingRequest(subject string, payload goja.ArrayBuffer) (goja.ArrayBuffer, error)

	ObjectGet(bucket string, objectName string) (goja.ArrayBuffer, error)
	ObjectPut(bucket string, objectName string, data goja.ArrayBuffer) error
}

type providerFactory func(workloadId string, allocator vmAllocator) hostServiceProvider

type vmAllocator interface {
	AllocateByteArray(workloadId string, bytes []byte) goja.ArrayBuffer
}

func NewScriptRunner(factory providerFactory) *ScriptRunner {
	return &ScriptRunner{
		factory: factory,
		scripts: make(map[string]*goja.Runtime),
	}
}

func (r *ScriptRunner) AllocateByteArray(workloadId string, bytes []byte) goja.ArrayBuffer {
	return r.GetVm(workloadId).NewArrayBuffer(bytes)
}

func (r *ScriptRunner) GetVm(workloadId string) *goja.Runtime {
	return r.scripts[workloadId]
}

func (r *ScriptRunner) AddScript(workloadId string, script string) error {
	vm := goja.New()
	vm.SetFieldNameMapper(goja.UncapFieldNameMapper())
	theProvider := r.factory(workloadId, r)
	vm.Set("host", theProvider)
	_, err := vm.RunString(script)
	if err != nil {
		return err
	}
	_, ok := goja.AssertFunction(vm.Get("run"))
	if !ok {
		return errors.New("no 'run' function in script")
	}

	r.scripts[workloadId] = vm

	return nil
}

func (r *ScriptRunner) RemoveScript(workloadId string) error {

	delete(r.scripts, workloadId)
	// TODO: this _should_ garbage collect nicely, but at some point we'll
	// want a test to make sure we don't leak when stopping

	return nil
}

func (r *ScriptRunner) TriggerScript(workloadId string, input []byte) ([]byte, error) {
	vm := r.scripts[workloadId]

	runFunc, ok := goja.AssertFunction(vm.Get("run"))
	if !ok {
		return nil, errors.New("no 'run' function found")
	}

	res, err := runFunc(goja.Undefined())
	if err != nil {
		return nil, err
	}
	out, ok := res.Export().(goja.ArrayBuffer)
	if !ok {
		outBytes, err := res.ToObject(vm).MarshalJSON()
		if err != nil {
			return nil, err
		}
		return outBytes, nil
	}
	return out.Bytes(), nil
}
