package hostservices

import (
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

const (
	testNamespace  = "testspace"
	testWorkload   = "testwork"
	testWorkloadId = "abc12346"
)

func setupSuite(_ testing.TB, port int) (*nats.Conn, func(tb testing.TB)) {

	svr, _ := server.NewServer(&server.Options{
		Port:      port,
		Host:      "0.0.0.0",
		JetStream: true,
	})
	svr.Start()

	nc, _ := nats.Connect(svr.ClientURL())

	// Return a function to teardown the test
	return nc, func(tb testing.TB) {
		svr.Shutdown()
	}
}

func TestBogusService(t *testing.T) {
	nc, teardownSuite := setupSuite(t, 4444)
	defer teardownSuite(t)

	server := NewHostServicesServer(nc, slog.Default())
	client := NewHostServicesClient(nc, 2*time.Second, testNamespace, testWorkload, testWorkloadId)

	boguss := bogusService{
		code:    99,
		message: "howdy",
		data:    []byte{1, 2, 3, 4, 5, 6},
	}

	_ = server.AddService("boguss", &boguss, []byte{})

	err := server.Start()
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
	nc, teardownSuite := setupSuite(t, 4445)
	defer teardownSuite(t)

	server := NewHostServicesServer(nc, slog.Default())
	client := NewHostServicesClient(nc, 2*time.Second, testNamespace, testWorkload, testWorkloadId)

	boguss := bogusService{
		code:    42,
		message: "howdy",
		data:    []byte{1, 2, 3, 4, 5, 6},
	}

	_ = server.AddService("boguss", &boguss, []byte{})

	err := server.Start()
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
	config  json.RawMessage
	code    uint
	message string
	data    []byte
}

func (b *bogusService) Initialize(config json.RawMessage) error {
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
