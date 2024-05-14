package hostservices

import (
	"encoding/json"
	"fmt"
)

const (
	headerCode    = "x-nex-hs-code"
	headerMessage = "x-nex-hs-msg"
	messageOk     = "OK"
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
	HandleRequest(namespace string,
		workloadId string,
		method string,
		workloadName string,
		metadata map[string]string,
		request []byte) (ServiceResult, error)
}
