//go:build !(linux && amd64)

package lib

import (
	"errors"

	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

type V8 struct{}

func (V8) Deploy() error { return nil }

func (V8) Execute(headers nats.Header, payload []byte) ([]byte, error) { return []byte{}, nil }

func (V8) Undeploy() error { return nil }

func (V8) Validate() error { return nil }

func InitNexExecutionProviderV8(params *agentapi.ExecutionProviderParams) (*V8, error) {
	return nil, errors.New("V8 is not supported on this platform")
}
