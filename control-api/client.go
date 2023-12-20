package controlapi

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// API subjects:
// $NEX.PING
// $NEX.PING.{node}
// $NEX.INFO.{namespace}.{node}
// $NEX.RUN.{namespace}.{node}
// $NEX.STOP.{namespace}.{node}

type apiClient struct {
	nc        *nats.Conn
	timeout   time.Duration
	namespace string
}

// Creates a new client to communicate with a group of NEX nodes
func NewApiClient(nc *nats.Conn, timeout time.Duration) *apiClient {
	return NewApiClientWithNamespace(nc, timeout, "default")
}

// Creates a new client to communicate with a group of NEX nodes all within a given namespace
func NewApiClientWithNamespace(nc *nats.Conn, timeout time.Duration, namespace string) *apiClient {
	return &apiClient{nc: nc, timeout: timeout, namespace: namespace}
}

// Attempts to stop a running workload. This can fail for a wide variety of reasons, the most common
// is likely to be security validation that prevents one issuer from issuing a stop request for
// another issuer's workload
func (api *apiClient) StopWorkload(stopRequest *StopRequest) (*StopResponse, error) {
	subject := fmt.Sprintf("%s.STOP.%s.%s", APIPrefix, api.namespace, stopRequest.TargetNode)
	bytes, err := api.performRequest(subject, stopRequest)
	if err != nil {
		return nil, err
	}

	var response StopResponse
	err = json.Unmarshal(bytes, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil

}

// Attempts to start a workload. The workload URI, at the moment, must always point to a NATS object store
// bucket in the form of `nats://{bucket}/{key}`
func (api *apiClient) StartWorkload(request *RunRequest) (*RunResponse, error) {
	subject := fmt.Sprintf("%s.RUN.%s.%s", APIPrefix, api.namespace, request.TargetNode)
	bytes, err := api.performRequest(subject, request)
	if err != nil {
		return nil, err
	}

	var response RunResponse
	err = json.Unmarshal(bytes, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// Requests information for a given node within the client's namespace
func (api *apiClient) NodeInfo(nodeId string) (*InfoResponse, error) {
	subject := fmt.Sprintf("%s.INFO.%s.%s", APIPrefix, api.namespace, nodeId)
	bytes, err := api.performRequest(subject, nil)
	if err != nil {
		return nil, err
	}

	var response InfoResponse
	err = json.Unmarshal(bytes, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// Attempts to list all nodes. Note that this operation returns all visible nodes regardless of
// namespace
func (api *apiClient) ListNodes() ([]PingResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), api.timeout)
	defer cancel()

	responses := make([]PingResponse, 0)

	sub, err := api.nc.Subscribe(api.nc.NewRespInbox(), func(m *nats.Msg) {
		env, err := extractEnvelope(m.Data)
		if err != nil {
			return
		}
		var resp PingResponse
		bytes, err := json.Marshal(env.Data)
		if err != nil {
			return
		}
		err = json.Unmarshal(bytes, &resp)
		if err != nil {
			return
		}
		responses = append(responses, resp)
	})
	if err != nil {
		return nil, nil
	}
	msg := nats.NewMsg(fmt.Sprintf("%s.PING", APIPrefix))
	msg.Reply = sub.Subject
	err = api.nc.PublishMsg(msg)
	if err != nil {
		return nil, err
	}

	<-ctx.Done()
	return responses, nil
}

// Helper that submits data, gets a standard envelope back, and returns the inner data
// payload as JSON
func (api *apiClient) performRequest(subject string, raw interface{}) ([]byte, error) {
	var bytes []byte
	var err error
	if raw == nil {
		bytes = []byte{}
	} else {
		bytes, err = json.Marshal(raw)
		if err != nil {
			return nil, err
		}
	}

	resp, err := api.nc.Request(subject, bytes, api.timeout)
	if err != nil {
		return nil, err
	}
	env, err := extractEnvelope(resp.Data)
	if err != nil {
		return nil, err
	}
	if env.Error != nil {
		return nil, fmt.Errorf("%v", env.Error)
	}
	return json.Marshal(env.Data)
}

func extractEnvelope(data []byte) (*Envelope, error) {
	var env Envelope
	err := json.Unmarshal(data, &env)
	if err != nil {
		return nil, err
	}
	return &env, nil
}
