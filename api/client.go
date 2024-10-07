package api

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/nex/node/actors"
)

var (
	DefaultRequestTimeout = 5 * time.Second
)

type ControlAPIClient struct {
	nc *nats.Conn
}

func NewControlApiClient(nc *nats.Conn) (*ControlAPIClient, error) {
	return &ControlAPIClient{
		nc: nc,
	}, nil
}

func (c *ControlAPIClient) Auction(tags []string) (*models.AuctionResponse, error) {
	req := models.AuctionRequest{}
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	msg, err := c.nc.Request(actors.AuctionSubject(), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	resp := new(models.AuctionResponse)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ControlAPIClient) Ping() (*models.PingResponse, error) {
	req := models.PingRequest{}
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	msg, err := c.nc.Request(actors.PingSubject(), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}
	resp := new(models.PingResponse)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ControlAPIClient) FindWorkload() (*models.FindWorkloadResponse, error) {
	req := models.FindWorkloadRequest{}
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	msg, err := c.nc.Request(actors.WorkloadPingSubject(), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	resp := new(models.FindWorkloadResponse)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ControlAPIClient) DirectPing(nodeId string) (*models.PingResponse, error) {
	req := models.PingRequest{}
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	msg, err := c.nc.Request(actors.DirectPingSubject(nodeId), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}
	resp := new(models.PingResponse)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ControlAPIClient) DeployWorkload(nodeId string) (*models.DeployResponse, error) {
	req := models.DeployRequest{}
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	msg, err := c.nc.Request(actors.DeploySubject(nodeId), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	resp := new(models.DeployResponse)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ControlAPIClient) UndeployWorkload(nodeId, workloadId string) (*models.UndeployResponse, error) {
	req := models.UndeployRequest{}
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	msg, err := c.nc.Request(actors.UndeploySubject(nodeId), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	resp := new(models.UndeployResponse)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ControlAPIClient) GetInfo(nodeId string) (*models.InfoResponse, error) {
	req := models.InfoRequest{}
	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	msg, err := c.nc.Request(actors.InfoSubject(nodeId), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	resp := new(models.InfoResponse)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ControlAPIClient) SetLameDuck(nodeId string, tag map[string]string) (*models.LameduckResponse, error) {
	if nodeId == "" && len(tag) != 1 {
		return nil, errors.New("invalid inputs to lameduck request")
	}

	req := models.LameduckRequest{
		NodeId: nodeId,
		Tag:    tag,
	}

	req_b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	msg, err := c.nc.Request(actors.LameduckSubject(nodeId), req_b, DefaultRequestTimeout)
	if err != nil {
		return nil, err
	}

	resp := new(models.LameduckResponse)
	err = json.Unmarshal(msg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
