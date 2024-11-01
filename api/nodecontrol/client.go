package nodecontrol

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	nodegen "github.com/synadia-io/nex/api/nodecontrol/gen"
	"github.com/synadia-io/nex/node/actors"
)

var (
	DefaultRequestTimeout = 5 * time.Second
)

type ControlAPIClient struct {
	nc     *nats.Conn
	logger *slog.Logger
}

func NewControlApiClient(nc *nats.Conn, logger *slog.Logger) (*ControlAPIClient, error) {
	return &ControlAPIClient{
		nc:     nc,
		logger: logger,
	}, nil
}

func (c *ControlAPIClient) Auction(tags []string) (*nodegen.AuctionResponseJson, error) {
	req := nodegen.AuctionRequestJson{}
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	msg, err := c.nc.Request(actors.AuctionSubject(), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	resp := new(nodegen.AuctionResponseJson)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ControlAPIClient) Ping() ([]*nodegen.NodePingResponseJson, error) {
	resp := []*nodegen.NodePingResponseJson{}
	pingRespInbox := nats.NewInbox()

	_, err := c.nc.Subscribe(pingRespInbox, func(m *nats.Msg) {
		envelope := new(actors.Envelope[nodegen.NodePingResponseJson])
		err := json.Unmarshal(m.Data, envelope)
		if err != nil {
			c.logger.Error("failed to unmarshal ping response", slog.Any("err", err), slog.String("data", string(m.Data)))
			return
		}
		resp = append(resp, &envelope.Data)
	})
	if err != nil {
		return nil, err
	}

	err = c.nc.PublishRequest(actors.PingSubject(), pingRespInbox, nil)
	if err != nil {
		return nil, err
	}

	time.Sleep(5 * time.Second)
	return resp, nil
}

func (c *ControlAPIClient) FindAgent(_type, namespace string) (*nodegen.AgentPingResponseJson, error) {
	msg, err := c.nc.Request(actors.AgentPingNamespaceRequestSubject(_type, namespace), nil, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	resp := new(nodegen.AgentPingResponseJson)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ControlAPIClient) DirectPing(nodeId string) (*nodegen.NodePingResponseJson, error) {
	msg, err := c.nc.Request(actors.DirectPingSubject(nodeId), nil, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}
	resp := new(nodegen.NodePingResponseJson)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ControlAPIClient) FindWorkload(_type, namespace, workloadId string) (*nodegen.AgentPingResponseJson, error) {
	msg, err := c.nc.Request(actors.AgentPingWorkloadRequestSubject(_type, namespace, workloadId), nil, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	resp := new(nodegen.AgentPingResponseJson)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ControlAPIClient) DeployWorkload(namespace, nodeId string, req nodegen.StartWorkloadRequestJson) (*nodegen.StartWorkloadResponseJson, error) {
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	msg, err := c.nc.Request(actors.DeployRequestSubject(namespace, nodeId), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	envelope := new(actors.Envelope[nodegen.StartWorkloadResponseJson])
	err = json.Unmarshal(msg.Data, envelope)
	if err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

func (c *ControlAPIClient) UndeployWorkload(nodeId, workloadId string, req nodegen.StopWorkloadRequestJson) (*nodegen.StopWorkloadResponseJson, error) {
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	msg, err := c.nc.Request(actors.UndeployRequestSubject(nodeId), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	envelope := new(actors.Envelope[nodegen.StopWorkloadResponseJson])
	err = json.Unmarshal(msg.Data, envelope)
	if err != nil {
		return nil, err
	}

	return &envelope.Data, nil
}

func (c *ControlAPIClient) GetInfo(nodeId, namespace string) (*nodegen.NodeInfoResponseJson, error) {
	msg, err := c.nc.Request(actors.InfoRequestSubject(nodeId, namespace), nil, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	resp := new(nodegen.NodeInfoResponseJson)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ControlAPIClient) SetLameDuck(nodeId string) (*nodegen.LameduckResponseJson, error) {
	msg, err := c.nc.Request(actors.LameduckSubject(nodeId), nil, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	resp := new(nodegen.LameduckResponseJson)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
