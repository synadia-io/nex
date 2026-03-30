package client

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"github.com/nats-io/nkeys"
	"github.com/nats-io/nuid"
	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/orbit.go/natsext"
)

const (
	defaultTimeout      = 60 * time.Second
	defaultStall        = 2 * time.Second
	defaultAuctionStall = 1 * time.Second
)

type NexClient interface {
	GetNexusPTags() (map[string]string, error)
	GetNodeInfo(nodeId string) (*models.NodeInfoResponse, error)
	SetLameduck(nodeId string, delay time.Duration, tag map[string]string) (*models.LameduckResponse, error)
	ListNodes(filter map[string]string) ([]*models.NodePingResponse, error)
	Auction(typ string, tags map[string]string) ([]*models.AuctionResponse, error)
	StartWorkload(deployId, name, desc, runRequest, typ string, lifecycle models.WorkloadLifecycle, pTags models.NodeTags) (*models.StartWorkloadResponse, error)
	StopWorkload(workloadId string) (*models.StopWorkloadResponse, error)
	ListWorkloads(filter []string) ([]*models.AgentListWorkloadsResponse, error)
	CloneWorkload(id string, tags map[string]string) (*models.StartWorkloadResponse, error)
}

type nexClient struct {
	ctx       context.Context
	cancel    context.CancelFunc
	nc        *nats.Conn
	namespace string
	errorID   *nuid.NUID

	// timeout configurations
	defaultTimeout          time.Duration
	startWorkloadTimeout    time.Duration
	requestManyStall        time.Duration
	auctionRequestManyStall time.Duration
}

func NewClient(ctx context.Context, nc *nats.Conn, namespace string, opts ...ClientOption) (NexClient, error) {
	var cancel context.CancelFunc
	if ctx == nil {
		ctx = context.Background()
	}

	client := &nexClient{
		nc:        nc,
		namespace: namespace,
		errorID:   nuid.New(),
		// Set default timeout values
		defaultTimeout:          defaultTimeout,
		startWorkloadTimeout:    time.Minute,
		requestManyStall:        defaultStall,
		auctionRequestManyStall: defaultAuctionStall,
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(client); err != nil {
			return nil, err
		}
	}

	// Set context timeout using configured default timeout
	if _, ok := ctx.Deadline(); !ok {
		ctx, cancel = context.WithTimeoutCause(ctx, client.defaultTimeout, errors.New("default nex client timeout exceeded"))
	}
	client.ctx = ctx
	client.cancel = cancel

	return client, nil
}

func (n *nexClient) GetNexusPTags() (map[string]string, error) {
	msgs, err := natsext.RequestMany(n.ctx, n.nc, models.PingPTagRequestSubject(n.namespace), []byte{}, natsext.RequestManyStall(n.requestManyStall))
	if errors.Is(err, nats.ErrNoResponders) || errors.Is(err, nats.ErrTimeout) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}

	var errs error
	resp := make(map[string]string)
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil && m.Data != nil && string(m.Data) != "null" {
			t := map[string]string{}
			err = json.Unmarshal(m.Data, &t)
			if err == nil {
				for k, v := range t {
					if _, ok := resp[k]; !ok {
						resp[k] = v
					}
				}
			}
		}
		errs = errors.Join(errs, err)
		return true
	})

	return resp, nil
}

func (n *nexClient) GetNodeInfo(nodeId string) (*models.NodeInfoResponse, error) {
	req := &models.NodeInfoRequest{}
	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to marshal node info request")
	}

	resp, err := n.nc.Request(models.NodeInfoRequestSubject(n.namespace, nodeId), reqB, n.defaultTimeout)
	if err != nil && !errors.Is(err, nats.ErrNoResponders) && !errors.Is(err, nats.ErrTimeout) {
		return nil, n.nexInternalError(err, "failed to request node info")
	}

	if err != nil || len(resp.Data) == 0 {
		return nil, n.nexNotFoundError(errors.New("node not found"), "node not found")
	}

	if nexErr := nexErrorFromMsg(resp); nexErr != nil {
		return nil, nexErr
	}

	infoResponse := new(models.NodeInfoResponse)
	err = json.Unmarshal(resp.Data, infoResponse)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to unmarshal node info response")
	}

	return infoResponse, nil
}

func (n *nexClient) SetLameduck(nodeId string, delay time.Duration, tag map[string]string) (*models.LameduckResponse, error) {
	req := &models.LameduckRequest{
		Delay: delay.String(),
		Tag:   tag,
	}

	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to marshal lameduck request")
	}

	respMsg, err := n.nc.Request(models.LameduckRequestSubject(n.namespace, nodeId), reqB, n.defaultTimeout)
	if err != nil && !errors.Is(err, nats.ErrNoResponders) {
		return nil, n.nexInternalError(err, "failed to request lameduck")
	}
	if errors.Is(err, nats.ErrNoResponders) {
		return &models.LameduckResponse{Success: false}, nil
	}

	if nexErr := nexErrorFromMsg(respMsg); nexErr != nil {
		return nil, nexErr
	}

	resp := new(models.LameduckResponse)
	err = json.Unmarshal(respMsg.Data, resp)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to unmarshal lameduck response")
	}

	return resp, nil
}

func (n *nexClient) ListNodes(filter map[string]string) ([]*models.NodePingResponse, error) {
	req := &models.NodePingRequest{
		Filter: filter,
	}

	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to marshal list nodes request")
	}

	msgs, err := natsext.RequestMany(n.ctx, n.nc, models.PingRequestSubject(n.namespace), reqB, natsext.RequestManyStall(n.requestManyStall))
	if errors.Is(err, nats.ErrNoResponders) || errors.Is(err, nats.ErrTimeout) {
		return []*models.NodePingResponse{}, nil
	}
	if err != nil {
		return nil, n.nexInternalError(err, "failed to request list nodes")
	}

	var errs error
	resp := []*models.NodePingResponse{}
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil && m.Data != nil && string(m.Data) != "null" {
			if m.Header.Get(micro.ErrorCodeHeader) != "" {
				return true
			}
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
		return nil, n.nexInternalError(err, "failed to marshal auction request")
	}

	msgs, err := natsext.RequestMany(n.ctx, n.nc, models.AuctionRequestSubject(n.namespace), auctionRequestB, natsext.RequestManyStall(n.auctionRequestManyStall))
	if errors.Is(err, nats.ErrNoResponders) {
		return []*models.AuctionResponse{}, nil
	}
	if err != nil {
		return nil, n.nexInternalError(err, "failed to request auction")
	}

	var errs error
	resp := []*models.AuctionResponse{}
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil {
			if m.Header.Get(micro.ErrorCodeHeader) != "" {
				return true
			}
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
		return nil, n.nexInternalError(err, "failed to marshal start workload request")
	}

	startResponseMsg, err := n.nc.Request(models.AuctionDeployRequestSubject(n.namespace, deployId), reqB, n.startWorkloadTimeout)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to request start workload")
	}

	if nexErr := nexErrorFromMsg(startResponseMsg); nexErr != nil {
		return nil, nexErr
	}

	startResponse := new(models.StartWorkloadResponse)
	err = json.Unmarshal(startResponseMsg.Data, startResponse)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to unmarshal start workload response")
	}

	return startResponse, nil
}

func (n *nexClient) StopWorkload(workloadId string) (*models.StopWorkloadResponse, error) {
	req := models.StopWorkloadRequest{
		Namespace: n.namespace,
	}

	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to marshal stop workload request")
	}

	msgs, err := natsext.RequestMany(n.ctx, n.nc, models.UndeployRequestSubject(n.namespace, workloadId), reqB, natsext.RequestManyStall(n.requestManyStall))
	if err != nil {
		return &models.StopWorkloadResponse{
			Id:           workloadId,
			Message:      err.Error(),
			Stopped:      false,
			WorkloadType: "",
		}, nil
	}

	ret := &models.StopWorkloadResponse{
		Id:           workloadId,
		Message:      string(models.GenericErrorsWorkloadNotFound),
		Stopped:      false,
		WorkloadType: "",
	}

	msgs(func(m *nats.Msg, e error) bool {
		if e == nil && m.Data != nil && string(m.Data) != "null" {
			if m.Header.Get(micro.ErrorCodeHeader) != "" {
				return true
			}
			var swresp models.StopWorkloadResponse
			err = json.Unmarshal(m.Data, &swresp)
			if err == nil {
				if swresp.Stopped {
					_ = json.Unmarshal(m.Data, ret)
					return false
				}
			}
		}
		return true
	})

	return ret, nil
}

func (n *nexClient) ListWorkloads(filter []string) ([]*models.AgentListWorkloadsResponse, error) {
	req := models.AgentListWorkloadsRequest{
		Namespace: n.namespace,
		Filter:    filter,
	}

	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to marshal list workloads request")
	}

	msgs, err := natsext.RequestMany(n.ctx, n.nc, models.NamespacePingRequestSubject(n.namespace), reqB, natsext.RequestManyStall(n.requestManyStall))
	if errors.Is(err, nats.ErrNoResponders) {
		return []*models.AgentListWorkloadsResponse{}, nil
	}
	if err != nil {
		return nil, n.nexInternalError(err, "failed to request list workloads")
	}

	var errs error
	resp := []*models.AgentListWorkloadsResponse{}
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil && m.Data != nil && string(m.Data) != "null" {
			if m.Header.Get(micro.ErrorCodeHeader) != "" {
				return true
			}
			t := new(models.AgentListWorkloadsResponse)
			err = json.Unmarshal(m.Data, t)
			if err == nil && len(*t) > 0 {
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
		return nil, n.nexInternalError(err, "failed to create clone keys")
	}

	tKpPub, err := tKp.PublicKey()
	if err != nil {
		return nil, n.nexInternalError(err, "failed to get clone public key")
	}

	cloneReq := models.CloneWorkloadRequest{
		Namespace:     n.namespace,
		NewTargetXkey: tKpPub,
	}

	cloneReqB, err := json.Marshal(cloneReq)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to marshal clone request")
	}

	genericNotFoundError := errors.New(string(models.GenericErrorsWorkloadNotFound))
	msgs, err := natsext.RequestMany(n.ctx, n.nc, models.CloneWorkloadRequestSubject(n.namespace, id), cloneReqB, natsext.RequestManyStall(n.requestManyStall))
	if errors.Is(err, nats.ErrNoResponders) {
		return nil, n.nexNotFoundError(genericNotFoundError, "workload not found")
	}
	if err != nil {
		return nil, n.nexInternalError(err, "workload not found")
	}

	var cloneResp *models.StartWorkloadRequest
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil && m.Data != nil && string(m.Data) != "null" {
			if m.Header.Get(micro.ErrorCodeHeader) != "" {
				return true
			}
			err = json.Unmarshal(m.Data, &cloneResp)
			if err != nil {
				return false
			}
		}
		return true
	})

	if cloneResp == nil {
		return nil, n.nexNotFoundError(errors.New(string(models.GenericErrorsWorkloadNotFound)), "workload not found")
	}

	aucResp, err := n.Auction(cloneResp.WorkloadType, tags)
	if err != nil {
		return nil, err
	}

	if len(aucResp) == 0 {
		return nil, n.nexNotFoundError(errors.New("no nodes available for placement"), "no nodes available for placement")
	}

	randomNode := aucResp[rand.Intn(len(aucResp))]
	swr, err := n.StartWorkload(randomNode.BidderId, cloneResp.Name, cloneResp.Description, cloneResp.RunRequest, cloneResp.WorkloadType, cloneResp.WorkloadLifecycle, tags)
	if err != nil {
		return nil, err
	}

	return swr, nil
}

func (n *nexClient) nexInternalError(err error, friendlyMsg string) *models.NexError {
	return models.NewNexError(n.errorID.Next(), err, friendlyMsg, http.StatusInternalServerError)
}

func (n *nexClient) nexBadRequestError(err error, friendlyMsg string) *models.NexError {
	return models.NewNexError(n.errorID.Next(), err, friendlyMsg, http.StatusBadRequest)
}

func (n *nexClient) nexNotFoundError(err error, friendlyMsg string) *models.NexError {
	return models.NewNexError(n.errorID.Next(), err, friendlyMsg, http.StatusNotFound)
}

func nexErrorFromMsg(msg *nats.Msg) *models.NexError {
	codeStr := msg.Header.Get(micro.ErrorCodeHeader)
	if codeStr == "" {
		return nil
	}

	friendlyMsg := msg.Header.Get(micro.ErrorHeader)
	httpStatus, _ := strconv.Atoi(codeStr)

	// Unmarshal failure is intentionally ignored — if the body is malformed,
	// body.Error will be empty and we fall back to using friendlyMsg from
	// the NATS micro error header as the error text.
	var body struct {
		ErrorID string `json:"error_id"`
		Error   string `json:"error"`
	}
	_ = json.Unmarshal(msg.Data, &body)

	rawErr := errors.New(body.Error)
	if body.Error == "" {
		rawErr = errors.New(friendlyMsg)
	}

	return models.NewNexError(body.ErrorID, rawErr, friendlyMsg, httpStatus)
}
