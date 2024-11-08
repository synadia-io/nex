package hostservices

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

const (
	headerCode    = "x-nex-hs-code"
	headerMessage = "x-nex-hs-msg"
	messageOk     = "OK"

	DefaultConnection      = "default"
	HostServicesConnection = "hostservices"

	HttpURLHeader               = "x-http-url"
	KeyValueKeyHeader           = "x-keyvalue-key"
	BucketContextHeader         = "x-context-bucket"
	MessagingSubjectHeader      = "x-subject"
	ObjectStoreObjectNameHeader = "x-object-name"
)

type ServiceResult struct {
	Code    uint
	Message string
	Data    []byte
}

func (r ServiceResult) Error() error {
	return fmt.Errorf("Error: %s (%d)", r.Message, r.Code)
}

func (r ServiceResult) IsError() bool {
	return r.Code != 200
}

func ServiceResultFail(code uint, message string) ServiceResult {
	return ServiceResult{
		Code:    code,
		Message: message,
		Data:    []byte{},
	}
}

func ServiceResultPass(code uint, message string, data []byte) ServiceResult {
	return ServiceResult{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

type HostService interface {
	Initialize(json.RawMessage) error

	HandleRequest(
		hsConnection *nats.Conn,
		namespace string,
		workloadId string,
		method string,
		workloadName string,
		metadata map[string]string,
		request []byte,
	) (ServiceResult, error)
}
