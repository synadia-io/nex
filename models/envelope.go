package models

import (
	"encoding/json"
	"strings"

	"github.com/nats-io/nats.go"
)

type Envelope[T any] struct {
	PayloadType string  `json:"type"`
	Data        T       `json:"data,omitempty"`
	Error       *string `json:"error,omitempty"`
	Code        int     `json:"code"`
}

func NewEnvelope[T any](dataType string, data T, code int, err *string) Envelope[T] {
	return Envelope[T]{
		PayloadType: dataType,
		Data:        data,
		Error:       err,
	}
}

func RespondEnvelope[T any](m *nats.Msg, dataType string, code int, data T, err string) {
	e := &err
	if len(strings.TrimSpace(err)) == 0 {
		e = nil
	}
	bytes, _ := json.Marshal(NewEnvelope(dataType, data, code, e))
	_ = m.Respond(bytes)
}
