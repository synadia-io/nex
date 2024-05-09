package hostservices

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	testNamespace  = "testspace"
	testWorkload   = "testwork"
	testWorkloadId = "abc12346"
)

func TestBogusService(t *testing.T) {

	nc, err := nats.Connect("0.0.0.0:4222")
	if err != nil {
		panic(err)
	}

	server := NewHostServicesServer(nc)
	client := NewHostServicesClient(nc, 2*time.Second, testNamespace, testWorkload, testWorkloadId)

	boguss := bogusService{
		code:    99,
		message: "howdy",
		data:    []byte{1, 2, 3, 4, 5, 6},
	}

	server.AddService("boguss", &boguss, make(map[string]string, 0))

	err = server.Start()
	if err != nil {
		panic(err)
	}

	result, err := client.PerformRpc(
		"boguss",
		"test",
		[]byte{9, 9, 9},
		make(map[string]string))

	if err != nil {
		panic(err)
	}

	if !slices.Equal(result.Data, []byte{1, 2, 3, 4, 5, 6}) {
		t.Fatalf("Data did not round trip properly: %+v", result.Data)
	}

	if result.Code != 99 {
		t.Fatalf("Code did not return properly: %d", result.Code)
	}
}

func TestServiceError(t *testing.T) {
	nc, err := nats.Connect("0.0.0.0:4222")
	if err != nil {
		panic(err)
	}

	server := NewHostServicesServer(nc)
	client := NewHostServicesClient(nc, 2*time.Second, testNamespace, testWorkload, testWorkloadId)

	boguss := bogusService{
		code:    42,
		message: "howdy",
		data:    []byte{1, 2, 3, 4, 5, 6},
	}

	server.AddService("boguss", &boguss, make(map[string]string, 0))

	err = server.Start()
	if err != nil {
		panic(err)
	}

	result, _ := client.PerformRpc(
		"boguss",
		"test",
		[]byte{9, 9, 9},
		make(map[string]string))

	if result.Code != 500 {
		t.Fatalf("Was supposed to get a 500, got %d", result.Code)
	}
	if result.Message != "Failed to execute host service method: faily fail fail" {
		t.Fatalf("Didn't get expected message. Got '%s'", result.Message)
	}
}

type bogusService struct {
	config  map[string]string
	code    uint
	message string
	data    []byte
}

func (b *bogusService) Initialize(config map[string]string) error {
	b.config = config

	return nil
}

func (b *bogusService) HandleRequest(namespace string,
	workloadId string,
	method string,
	workloadName string,
	metadata map[string]string,
	request []byte) (ServiceResult, error) {

	if b.code != 42 {
		return ServiceResult{
			Code:    b.code,
			Message: b.message,
			Data:    b.data,
		}, nil
	}

	return ServiceResult{}, errors.New("faily fail fail")

}
