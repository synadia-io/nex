package client

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/nats-io/nuid"
	"github.com/synadia-io/orbit.go/natsext"
	"github.com/synadia-labs/nex/models"
)

type nexClient struct {
	nc        *nats.Conn
	namespace string
}

func NewClient(nc *nats.Conn, namespace string) *nexClient {
	return &nexClient{
		nc:        nc,
		namespace: namespace,
	}
}

func (n *nexClient) GetNodeInfo(nodeId string) (*models.NodeInfoResponse, error) {
	req := &models.NodeInfoRequest{}
	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := n.nc.Request(models.NodeInfoRequestSubject(n.namespace, nodeId), reqB, 10*time.Second)
	if err != nil && !errors.Is(err, nats.ErrNoResponders) && !errors.Is(err, nats.ErrTimeout) {
		return nil, err
	}

	if err != nil || len(resp.Data) == 0 {
		return nil, errors.New("node not found")
	}

	infoResponse := new(models.NodeInfoResponse)
	err = json.Unmarshal(resp.Data, infoResponse)
	if err != nil {
		return nil, err
	}

	return infoResponse, nil
}

func (n *nexClient) SetLameduck(nodeId string, delay time.Duration) (*models.LameduckResponse, error) {
	req := &models.LameduckRequest{
		Delay: delay.String(),
	}

	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respMsg, err := n.nc.Request(models.LameduckRequestSubject(n.namespace, nodeId), reqB, 3*time.Second)
	if err != nil && !errors.Is(err, nats.ErrNoResponders) {
		return nil, err
	}
	if errors.Is(err, nats.ErrNoResponders) {
		return &models.LameduckResponse{Success: false}, nil
	}

	resp := new(models.LameduckResponse)
	err = json.Unmarshal(respMsg.Data, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (n *nexClient) ListNodes(filter map[string]string) ([]*models.NodePingResponse, error) {
	req := &models.NodePingRequest{
		Filter: filter,
	}

	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	msgs, err := natsext.RequestMany(ctx, n.nc, models.PingRequestSubject(n.namespace), reqB, natsext.RequestManyStall(500*time.Millisecond))
	if errors.Is(err, nats.ErrNoResponders) || errors.Is(err, nats.ErrTimeout) {
		return []*models.NodePingResponse{}, nil
	}
	if err != nil {
		return nil, err
	}

	var errs error
	resp := []*models.NodePingResponse{}
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil {
			t := new(models.NodePingResponse)
			err = json.Unmarshal(m.Data, t)
			if err == nil {
				resp = append(resp, t)
			}
		}
		errs = errors.Join(errs, err)
		return true
	})

	return resp, nil
}

func (n *nexClient) Auction(typ string, tags map[string]string) ([]*models.AuctionResponse, error) {
	auctionRequest := &models.AuctionRequest{
		AgentType: typ,
		AuctionId: nuid.New().Next(),
		Tags:      tags,
	}

	auctionRequestB, err := json.Marshal(auctionRequest)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	msgs, err := natsext.RequestMany(ctx, n.nc, models.AuctionRequestSubject(n.namespace), auctionRequestB, natsext.RequestManyStall(500*time.Millisecond))
	if errors.Is(err, nats.ErrNoResponders) {
		return []*models.AuctionResponse{}, nil
	}
	if err != nil {
		return nil, err
	}

	var errs error
	resp := []*models.AuctionResponse{}
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil {
			t := new(models.AuctionResponse)
			err = json.Unmarshal(m.Data, t)
			if err == nil {
				resp = append(resp, t)
			}
		}
		errs = errors.Join(errs, err)
		return true
	})

	return resp, nil
}

func (n *nexClient) StartWorkload(deployId, name, desc, runRequest, typ string, lifecycle models.WorkloadLifecycle) (*models.StartWorkloadResponse, error) {
	req := &models.StartWorkloadRequest{
		Namespace:         n.namespace,
		Name:              name,
		Description:       desc,
		RunRequest:        runRequest,
		WorkloadLifecycle: lifecycle,
		WorkloadType:      typ,
	}

	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var startResponseMsg *nats.Msg
	if nkeys.IsValidPublicServerKey(deployId) {
		startResponseMsg, err = n.nc.Request(models.DirectDeploySubject(deployId), reqB, 10*time.Second)
		if err != nil {
			return nil, err
		}
	} else {
		startResponseMsg, err = n.nc.Request(models.AuctionDeployRequestSubject(n.namespace, deployId), reqB, 10*time.Second)
		if err != nil {
			return nil, err
		}
	}

	startResponse := new(models.StartWorkloadResponse)
	err = json.Unmarshal(startResponseMsg.Data, startResponse)
	if err != nil {
		return nil, err
	}

	return startResponse, nil
}

func (n *nexClient) StopWorkload(workloadId string) (*models.StopWorkloadResponse, error) {
	req := models.StopWorkloadRequest{
		Namespace: n.namespace,
	}

	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := n.nc.Request(models.UndeployRequestSubject(n.namespace, workloadId), reqB, 10*time.Second)
	if err != nil {
		return nil, err
	}

	stopResponse := new(models.StopWorkloadResponse)
	err = json.Unmarshal(resp.Data, stopResponse)
	if err != nil {
		return nil, err
	}
	return stopResponse, nil
}

func (n *nexClient) ListWorkloads(filter []string) ([]*models.AgentListWorkloadsResponse, error) {
	req := models.AgentListWorkloadsRequest{
		Namespace: n.namespace,
		Filter:    filter,
	}

	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	msgs, err := natsext.RequestMany(ctx, n.nc, models.NamespacePingRequestSubject(n.namespace), reqB, natsext.RequestManyStall(1000*time.Millisecond))
	if errors.Is(err, nats.ErrNoResponders) {
		return []*models.AgentListWorkloadsResponse{}, nil
	}
	if err != nil {
		return nil, err
	}

	var errs error
	resp := []*models.AgentListWorkloadsResponse{}
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil {
			t := new(models.AgentListWorkloadsResponse)
			err = json.Unmarshal(m.Data, t)
			if err == nil {
				resp = append(resp, t)
			}
		}
		errs = errors.Join(errs, err)
		return true
	})

	return resp, nil
}

func (n *nexClient) CloneWorkload(id string, tags map[string]string) (*models.StartWorkloadResponse, error) {
	tKp, err := nkeys.CreateCurveKeys()
	if err != nil {
		return nil, err
	}

	tKpPub, err := tKp.PublicKey()
	if err != nil {
		return nil, err
	}

	cloneReq := models.CloneWorkloadRequest{
		Namespace:     n.namespace,
		NewTargetXkey: tKpPub,
	}

	cloneReqB, err := json.Marshal(cloneReq)
	if err != nil {
		return nil, err
	}

	resp, err := n.nc.Request(models.CloneWorkloadRequestSubject(n.namespace, id), cloneReqB, 5*time.Second)
	if err != nil && !errors.Is(err, nats.ErrTimeout) {
		return nil, err
	}

	if errors.Is(err, nats.ErrTimeout) {
		return nil, errors.New("workload not found")
	}

	cloneResp := new(models.StartWorkloadRequest)
	err = json.Unmarshal(resp.Data, cloneResp)
	if err != nil {
		return nil, err
	}

	aucResp, err := n.Auction(cloneResp.WorkloadType, tags)
	if err != nil {
		return nil, err
	}

	randomNode := aucResp[rand.Intn(len(aucResp))]

	return n.StartWorkload(randomNode.BidderId, cloneResp.Name, cloneResp.Description, cloneResp.RunRequest, cloneResp.WorkloadType, cloneResp.WorkloadLifecycle)
}
