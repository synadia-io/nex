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
	if err != nil && !errors.Is(err, nats.ErrNoResponders) {
		return nil, err
	}

	if errors.Is(err, nats.ErrNoResponders) {
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
		return nil, errors.New("no nodes found")
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

	resp := []*models.NodePingResponse{}
	var last time.Time
	var errs error

	inbox := nats.NewInbox()
	_, err = n.nc.Subscribe(inbox, func(m *nats.Msg) {
		last = time.Now()
		t := new(models.NodePingResponse)
		err := json.Unmarshal(m.Data, t)
		if err != nil {
			errs = errors.Join(errs, err)
			return
		}
		resp = append(resp, t)
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	go func() {
		for range time.Tick(250 * time.Millisecond) {
			select {
			case <-ctx.Done():
				return
			default:
				if last.IsZero() {
					continue
				}
				if time.Since(last) > time.Second {
					cancel()
				}
			}
		}
	}()

	err = n.nc.PublishRequest(models.PingRequestSubject(n.namespace), inbox, reqB)
	if err != nil && !errors.Is(err, nats.ErrNoResponders) {
		return nil, err
	}
	if errors.Is(err, nats.ErrNoResponders) {
		return nil, errors.New("no nodes found")
	}

	<-ctx.Done()

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

	resp := []*models.AuctionResponse{}
	var last time.Time
	var errs error

	respInbox := nats.NewInbox()
	sub, err := n.nc.Subscribe(respInbox, func(m *nats.Msg) {
		last = time.Now()
		t := new(models.AuctionResponse)
		err := json.Unmarshal(m.Data, t)
		if err != nil {
			errs = errors.Join(errs, err)
			return
		}
		resp = append(resp, t)
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = sub.Unsubscribe()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	go func() {
		for range time.Tick(250 * time.Millisecond) {
			select {
			case <-ctx.Done():
				return
			default:
				if last.IsZero() {
					continue
				}
				if time.Since(last) > time.Second {
					cancel()
				}
			}
		}
	}()

	err = n.nc.PublishRequest(models.AuctionRequestSubject(n.namespace), respInbox, auctionRequestB)
	if err != nil {
		return nil, err
	}

	<-ctx.Done()

	if errors.Is(err, nats.ErrNoResponders) || len(resp) == 0 {
		return nil, errors.New("no nodes found to satisfy workload request")
	}

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

	resp := []*models.AgentListWorkloadsResponse{}

	var last time.Time
	var errs error

	inbox := nats.NewInbox()
	_, err = n.nc.Subscribe(inbox, func(m *nats.Msg) {
		last = time.Now()
		t := new(models.AgentListWorkloadsResponse)
		err := json.Unmarshal(m.Data, t)
		if err != nil {
			errs = errors.Join(errs, err)
			return
		}
		resp = append(resp, t)
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	go func() {
		for range time.Tick(250 * time.Millisecond) {
			select {
			case <-ctx.Done():
				return
			default:
				if last.IsZero() {
					continue
				}
				if time.Since(last) > time.Second {
					cancel()
				}
			}
		}
	}()

	err = n.nc.PublishRequest(models.NamespacePingRequestSubject(n.namespace), inbox, reqB)
	if err != nil && !errors.Is(err, nats.ErrNoResponders) {
		return nil, err
	}

	<-ctx.Done()

	if errors.Is(err, nats.ErrNoResponders) {
		return nil, errors.New("no workloads found")
	}
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
