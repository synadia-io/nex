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

func NewApiClient(nc *nats.Conn, timeout time.Duration) *apiClient {
	return &apiClient{nc: nc, timeout: timeout, namespace: "default"}
}

func NewApiClientWithNamespace(nc *nats.Conn, timeout time.Duration, namespace string) *apiClient {
	return &apiClient{nc: nc, timeout: timeout, namespace: namespace}
}

func (api *apiClient) StartWorkload(request *RunRequest) (*RunResponse, error) {
	var response RunResponse
	bytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := api.nc.Request(fmt.Sprintf("%s.RUN.%s.%s", APIPrefix, api.namespace, request.TargetNode), bytes, api.timeout)
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
	respBytes, err := json.Marshal(env.Data)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(respBytes, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func (api *apiClient) NodeInfo(nodeId string) (*InfoResponse, error) {
	var response InfoResponse
	resp, err := api.nc.Request(fmt.Sprintf("%s.INFO.%s.%s", APIPrefix, api.namespace, nodeId), []byte{}, api.timeout)
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
	bytes, err := json.Marshal(env.Data)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bytes, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

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
	api.nc.PublishMsg(msg)

	<-ctx.Done()
	return responses, nil
}

func extractEnvelope(data []byte) (*Envelope, error) {
	var env Envelope
	err := json.Unmarshal(data, &env)
	if err != nil {
		return nil, err
	}
	return &env, nil
}
