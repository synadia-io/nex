package api

import (
	"time"

	"github.com/nats-io/nats.go"
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

func (c *ControlAPIClient) Auction(tags []string) (string, error) {
	auctionPayload := []byte{}
	msg, err := c.nc.Request(actors.AuctionSubject(), auctionPayload, DefaultRequestTimeout)
	if err != nil {
		return "", err
	}
	return string(msg.Data), nil
}

func (c *ControlAPIClient) Ping() error {
	pingPayload := []byte{}
	_, err := c.nc.Request(actors.PingSubject(), pingPayload, DefaultRequestTimeout)
	if err != nil {
		return err
	}
	return nil
}

func (c *ControlAPIClient) FindWorkload() (string, error) {
	workloadPingPayload := []byte{}
	msg, err := c.nc.Request(actors.WorkloadPingSubject(), workloadPingPayload, DefaultRequestTimeout)
	if err != nil {
		return "", err
	}
	return string(msg.Data), nil
}

func (c *ControlAPIClient) DirectPing(nodeId string) error {
	directPingPayload := []byte{}
	_, err := c.nc.Request(actors.DirectPingSubject(nodeId), directPingPayload, DefaultRequestTimeout)
	if err != nil {
		return err
	}

	return nil
}

func (c *ControlAPIClient) DeployWorkload(nodeId string) error {
	deployPayload := []byte{}
	_, err := c.nc.Request(actors.DeploySubject(nodeId), deployPayload, DefaultRequestTimeout)
	if err != nil {
		return err
	}

	return nil
}

func (c *ControlAPIClient) UndeployWorkload(nodeId, workloadId string) error {
	unDeployPayload := []byte(workloadId)
	_, err := c.nc.Request(actors.UndeploySubject(nodeId), unDeployPayload, DefaultRequestTimeout)
	if err != nil {
		return err
	}

	return nil
}

func (c *ControlAPIClient) GetInfo(nodeId string) (string, error) {
	infoPayload := []byte{}
	msg, err := c.nc.Request(actors.InfoSubject(nodeId), infoPayload, DefaultRequestTimeout)
	if err != nil {
		return "", err
	}

	return string(msg.Data), nil
}

func (c *ControlAPIClient) SetLameDuck(nodeId string) error {
	lameDuckPayload := []byte{}
	_, err := c.nc.Request(actors.LameduckSubject(nodeId), lameDuckPayload, DefaultRequestTimeout)
	if err != nil {
		return err
	}

	return nil
}
