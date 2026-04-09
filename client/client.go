package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	Auction(namespace, typ string, tags map[string]string) ([]*models.AuctionResponse, error)
	StartWorkload(deployId string, req *models.StartWorkloadRequest) (*models.StartWorkloadResponse, error)
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
	if err != nil {
		return nil, err
	}

	var errs error
	resp := make(map[string]string)
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil && m.Data != nil && string(m.Data) != "null" {
			if nexErr := nexErrorFromMsg(m); nexErr != nil {
				errs = errors.Join(errs, nexErr)
				return true
			}
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
		if err != nil && !errors.Is(err, nats.ErrNoResponders) {
			errs = errors.Join(errs, err)
		}
		return true
	})

	return resp, errs
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
	if err != nil {
		return nil, n.nexInternalError(err, "failed to request list nodes")
	}

	var errs error
	resp := []*models.NodePingResponse{}
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil && m.Data != nil && string(m.Data) != "null" {
			if nexErr := nexErrorFromMsg(m); nexErr != nil {
				errs = errors.Join(errs, nexErr)
				return true
			}
			t := new(models.NodePingResponse)
			err = json.Unmarshal(m.Data, t)
			if err == nil {
				resp = append(resp, t)
			}
		}
		if err != nil && !errors.Is(err, nats.ErrNoResponders) {
			errs = errors.Join(errs, err)
		}
		return true
	})

	return resp, errs
}

func (n *nexClient) Auction(namespace, typ string, tags map[string]string) ([]*models.AuctionResponse, error) {
	auctionRequest := &models.AuctionRequest{
		AgentType: typ,
		AuctionId: nuid.New().Next(),
		Tags:      tags,
	}

	auctionRequestB, err := json.Marshal(auctionRequest)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to marshal auction request")
	}

	msgs, err := natsext.RequestMany(n.ctx, n.nc, models.AuctionRequestSubject(namespace), auctionRequestB, natsext.RequestManyStall(n.auctionRequestManyStall))
	if err != nil {
		return nil, n.nexInternalError(err, "failed to request auction")
	}

	var errs error
	resp := []*models.AuctionResponse{}
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil {
			if nexErr := nexErrorFromMsg(m); nexErr != nil {
				errs = errors.Join(errs, nexErr)
				return true
			}
			t := new(models.AuctionResponse)
			err = json.Unmarshal(m.Data, t)
			if err == nil {
				resp = append(resp, t)
			}
		}
		if err != nil && !errors.Is(err, nats.ErrNoResponders) {
			errs = errors.Join(errs, err)
		}
		return true
	})

	return resp, errs
}

func (n *nexClient) StartWorkload(deployId string, req *models.StartWorkloadRequest) (*models.StartWorkloadResponse, error) {
	if req == nil {
		return nil, n.nexBadRequestError(errors.New("nil request"), "start workload request must not be nil")
	}

	// Shallow-copy so defaulting Tags does not mutate the caller's struct.
	// This matters for call sites like CloneWorkload that pass the agent's
	// StartWorkloadRequest through verbatim — we must not silently poke at
	// fields on a value the caller still holds a reference to.
	local := *req
	if local.Tags == nil {
		local.Tags = make(models.NodeTags)
	}

	reqB, err := json.Marshal(&local)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to marshal start workload request")
	}

	startResponseMsg, err := n.nc.Request(models.AuctionDeployRequestSubject(local.Namespace, deployId), reqB, n.startWorkloadTimeout)
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
	// targetNS is the namespace the workload actually lives in. For user
	// callers it is the caller's own namespace. For system callers we must
	// discover it because system is administrative and never hosts
	// workloads itself (except in rare explicit system-namespace starts).
	targetNS := n.namespace

	if n.namespace == models.SystemNamespace {
		summaries, err := n.ListWorkloads([]string{workloadId})
		if err != nil {
			return nil, n.nexInternalError(err, "failed to discover workload namespace")
		}

		var resolved []string
		for _, agentResp := range summaries {
			for _, summary := range *agentResp {
				if summary.Id != workloadId {
					continue
				}
				if summary.Namespace == nil {
					return nil, n.nexInternalError(
						errors.New("owning namespace not reported"),
						fmt.Sprintf("cannot stop workload %s as system user: owning namespace could not be determined (nexlet does not report namespace in list response); retry with --namespace set to the owning namespace", workloadId))
				}
				already := false
				for _, existing := range resolved {
					if existing == *summary.Namespace {
						already = true
						break
					}
				}
				if !already {
					resolved = append(resolved, *summary.Namespace)
				}
			}
		}

		switch len(resolved) {
		case 0:
			return nil, n.nexNotFoundError(errors.New(string(models.GenericErrorsWorkloadNotFound)), "workload not found")
		case 1:
			targetNS = resolved[0]
		default:
			// Don't echo the resolved namespace names in the error
			// message — even though only system callers reach this branch
			// today, the error propagates through NATS micro response
			// headers and logs where the audience is broader. The
			// operator can list workloads themselves to investigate.
			return nil, n.nexInternalError(
				errors.New("ambiguous workload id"),
				fmt.Sprintf("workload id %s is ambiguous: it matches workloads in multiple namespaces; use --namespace to specify which one to stop", workloadId))
		}
	}

	req := models.StopWorkloadRequest{
		Namespace: targetNS,
	}

	reqB, err := json.Marshal(req)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to marshal stop workload request")
	}

	msgs, err := natsext.RequestMany(n.ctx, n.nc, models.UndeployRequestSubject(targetNS, workloadId), reqB, natsext.RequestManyStall(n.requestManyStall))
	if err != nil {
		return nil, n.nexInternalError(err, "failed to request stop workload")
	}

	ret := &models.StopWorkloadResponse{
		Id:           workloadId,
		Message:      string(models.GenericErrorsWorkloadNotFound),
		Stopped:      false,
		WorkloadType: "",
	}

	var errs error
	msgs(func(m *nats.Msg, e error) bool {
		if e == nil && m.Data != nil && string(m.Data) != "null" {
			if nexErr := nexErrorFromMsg(m); nexErr != nil {
				errs = errors.Join(errs, nexErr)
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
		if e != nil && !errors.Is(e, nats.ErrNoResponders) {
			errs = errors.Join(errs, e)
		}
		return true
	})

	return ret, errs
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
	if err != nil {
		return nil, n.nexInternalError(err, "failed to request list workloads")
	}

	var errs error
	resp := []*models.AgentListWorkloadsResponse{}
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil && m.Data != nil && string(m.Data) != "null" {
			if nexErr := nexErrorFromMsg(m); nexErr != nil {
				errs = errors.Join(errs, nexErr)
				return true
			}
			t := new(models.AgentListWorkloadsResponse)
			err = json.Unmarshal(m.Data, t)
			if err == nil && len(*t) > 0 {
				resp = append(resp, t)
			}
		}
		if err != nil && !errors.Is(err, nats.ErrNoResponders) {
			errs = errors.Join(errs, err)
		}
		return true
	})

	return resp, errs
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

	// The clone fetch uses n.namespace because the caller has not yet
	// learned the target namespace — the whole point of this round-trip is
	// to discover the workload's owning namespace along with its full
	// definition. The node's handleCloneWorkload bypass for
	// models.SystemNamespace is load-bearing here for cross-namespace
	// clones by a system operator; a user caller can only fetch workloads
	// in their own namespace (handlers.go enforces this).
	cloneReq := models.CloneWorkloadRequest{
		Namespace:     n.namespace,
		NewTargetXkey: tKpPub,
	}

	cloneReqB, err := json.Marshal(cloneReq)
	if err != nil {
		return nil, n.nexInternalError(err, "failed to marshal clone request")
	}

	msgs, err := natsext.RequestMany(n.ctx, n.nc, models.CloneWorkloadRequestSubject(n.namespace, id), cloneReqB, natsext.RequestManyStall(n.requestManyStall))
	if err != nil {
		return nil, n.nexInternalError(err, "workload not found")
	}

	var cloneResp *models.StartWorkloadRequest
	var cloneErrs error
	msgs(func(m *nats.Msg, err error) bool {
		if err == nil && m.Data != nil && string(m.Data) != "null" {
			if nexErr := nexErrorFromMsg(m); nexErr != nil {
				cloneErrs = errors.Join(cloneErrs, nexErr)
				return true
			}
			err = json.Unmarshal(m.Data, &cloneResp)
			if err != nil {
				return false
			}
		}
		if err != nil && !errors.Is(err, nats.ErrNoResponders) {
			cloneErrs = errors.Join(cloneErrs, err)
		}
		return true
	})

	if cloneResp == nil && cloneErrs != nil {
		return nil, cloneErrs
	}

	if cloneResp == nil {
		return nil, n.nexNotFoundError(errors.New(string(models.GenericErrorsWorkloadNotFound)), "workload not found")
	}

	// An empty namespace on the clone response means the agent did not
	// report the workload's owning namespace — typically an older nexlet
	// that pre-dates the WorkloadSummary.Namespace contract. Without a
	// target namespace we would publish the auction on a malformed subject
	// and the failure would surface as an unhelpful "no nodes available"
	// error. Fail fast with a clear message instead.
	if cloneResp.Namespace == "" {
		return nil, n.nexInternalError(
			errors.New("clone response missing namespace"),
			"cannot clone workload: owning namespace not reported by the agent")
	}

	// The clone must be deployed into the workload's actual owning
	// namespace, not the caller's namespace. For a system operator cloning
	// a default-namespace workload, this means the auction and deploy
	// target `default`. The system credential masquerades at the subject
	// layer — publishing on default.* control subjects while authenticated
	// as system — which the node accepts because subject-body namespaces
	// match. cloneResp already carries the authoritative Namespace from
	// the agent's state, so we reuse it verbatim and only override the
	// placement tags the caller passed to CloneWorkload.
	aucResp, err := n.Auction(cloneResp.Namespace, cloneResp.WorkloadType, tags)
	if len(aucResp) == 0 {
		if err != nil {
			return nil, err
		}
		return nil, n.nexNotFoundError(errors.New("no nodes available for placement"), "no nodes available for placement")
	}

	randomNode := aucResp[rand.Intn(len(aucResp))]
	cloneResp.Tags = tags
	swr, err := n.StartWorkload(randomNode.BidderId, cloneResp)
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
