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

const defaultTimeout = 60 * time.Second

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

	resp, err := n.nc.Request(models.NodeInfoRequestSubject(n.namespace, nodeId), reqB, defaultTimeout)
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

func (n *nexClient) StartWorkload(deployId, name, desc, runRequest, typ string, lifecycle models.WorkloadLifecycle, pTags models.NodeTags) (*models.StartWorkloadResponse, error) {
	if pTags == nil {
		pTags = make(models.NodeTags)
	}

	req := &models.StartWorkloadRequest{
		Namespace:         n.namespace,
		Name:              name,
		Description:       desc,
		RunRequest:        runRequest,
		WorkloadLifecycle: lifecycle,
		WorkloadType:      typ,
		Tags:              pTags,
	}

	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	startResponseMsg, err := n.nc.Request(models.AuctionDeployRequestSubject(n.namespace, deployId), reqB, time.Minute)
	if err != nil {
		return nil, err
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

	// BUG: this is coming back empty and shouldnt be
	_, err = n.nc.Request(models.UndeployRequestSubject(n.namespace, workloadId), reqB, defaultTimeout)
	if err != nil {
		return nil, err
	}

	return &models.StopWorkloadResponse{
		Id:      workloadId,
		Stopped: true,
	}, nil
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

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	msgs, err := natsext.RequestMany(ctx, n.nc, models.NamespacePingRequestSubject(n.namespace), reqB, natsext.RequestManyStall(2000*time.Millisecond))
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

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	msgs, err := natsext.RequestMany(ctx, n.nc, models.CloneWorkloadRequestSubject(n.namespace, id), cloneReqB, natsext.RequestManyStall(2000*time.Millisecond))
	if errors.Is(err, nats.ErrNoResponders) {
		return nil, errors.New("workload not found")
	}
	if err != nil {
		return nil, errors.New("workload not found")
	}
	if errors.Is(err, nats.ErrTimeout) {
		return nil, errors.New("workload not found")
	}

	var cloneResp *models.StartWorkloadRequest
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil && m.Data != nil && string(m.Data) != "null" {
			err = json.Unmarshal(m.Data, &cloneResp)
			if err != nil {
				return false
			}
		}
		return true
	})

	if cloneResp == nil {
		return nil, errors.New("workload not found")
	}

	aucResp, err := n.Auction(cloneResp.WorkloadType, tags)
	if err != nil {
		return nil, err
	}

	if len(aucResp) == 0 {
		return nil, errors.New("no nodes available for placement")
	}

	randomNode := aucResp[rand.Intn(len(aucResp))]
	swr, err := n.StartWorkload(randomNode.BidderId, cloneResp.Name, cloneResp.Description, cloneResp.RunRequest, cloneResp.WorkloadType, cloneResp.WorkloadLifecycle, tags)
	if err != nil {
		return nil, err
	}

	return swr, nil
}
