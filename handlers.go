package nex

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"disorder.dev/shandler"
	"github.com/synadia-io/orbit.go/natsext"
	"github.com/synadia-labs/nex/models"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func (n *NexNode) handlePing() func(micro.Request) {
	return func(r micro.Request) {
		rep := new(models.NodePingRequest)
		err := json.Unmarshal(r.Data(), rep)
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal ping request")
			return
		}

		for k, v := range rep.Filter {
			if tV, ok := n.tags[k]; !ok || tV != v {
				return
			}
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, "100", "failed to get public key from keypair")
			return
		}

		pubXKey, err := n.nodeXKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, "100", "failed to get public xkey from xkeypair")
			return
		}

		err = r.RespondJSON(models.NodePingResponse{
			AgentCount: n.agentCount(),
			NodeId:     pubKey,
			Tags:       n.tags,
			Uptime:     time.Since(n.startTime).String(),
			Version:    n.version,
			Xkey:       pubXKey,
			State:      n.nodeState,
		})
		if err != nil {
			n.logger.Error("failed to respond to node info request", slog.String("err", err.Error()))
			return
		}
	}
}

func (n *NexNode) handleLameduck() func(micro.Request) {
	return func(r micro.Request) {
		req := new(models.LameduckRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal lameduck request")
			return
		}

		delay, err := time.ParseDuration(req.Delay)
		if err != nil {
			n.handlerError(r, err, "100", "failed to parse lameduck delay")
			return
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, "100", "failed to get public key from keypair")
			return
		}

		ldReq := models.LameduckRequest{
			Delay: delay.String(),
		}

		ldReqB, err := json.Marshal(ldReq)
		if err != nil {
			n.handlerError(r, err, "100", "failed to marshal lameduck request")
			return
		}

		// TODO: Adds agentid to lameduck response
		var errs error
		msgs, err := natsext.RequestMany(n.ctx, n.nc, models.AgentAPISetLameduckSubject(pubKey), ldReqB, natsext.RequestManyMaxMessages(n.regs.Count()))
		if err == nil {
			msgs(func(m *nats.Msg, err error) bool {
				if err == nil {
					agentId := m.Header.Get("agentId")
					if agentId == "" {
						errs = errors.Join(errs, errors.New("failed to get agentId from header"))
						return true
					}

					t := new(models.LameduckResponse)
					err = json.Unmarshal(m.Data, t)
					if err == nil {
						errs = errors.Join(errs, err)
						return true
					}

					err = emitSystemEvent(n.nc, n.nodeKeypair, &models.AgentLameduckSetEvent{
						Success: t.Success,
					})
					if err != nil {
						errs = errors.Join(errs, err)
					}
				}

				errs = errors.Join(errs, err)
				return true
			})
		} else {
			errs = errors.Join(errs, err)
		}

		if errs != nil {
			n.logger.Error("error gathering agent responses", slog.Any("errs", errs))
		}
		n.enterLameduck(delay)

		n.logger.Info("node entering lameduck mode", slog.Any("shutdown_at", time.Now().Add(delay).Format(time.DateTime)))
		n.tags[models.TagLameDuck] = "true"
		err = r.RespondJSON(models.LameduckResponse{
			Success: true,
		})
		if err != nil {
			n.logger.Error("failed to respond to lameduck request", slog.String("err", err.Error()))
			return
		}
	}
}

func (n *NexNode) handleNodeInfo() func(micro.Request) {
	return func(r micro.Request) {
		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, "100", "failed to get public key from keypair")
			return
		}
		pubXKey, err := n.nodeXKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, "100", "failed to get public xkey from xkeypair")
			return
		}

		var errs error
		as := models.AgentSummaries{}
		msgs, err := natsext.RequestMany(n.ctx, n.nc, models.AgentAPIPingAllSubject(pubKey), nil, natsext.RequestManyMaxMessages(n.regs.Count()))
		if err == nil {
			msgs(func(m *nats.Msg, err error) bool {
				if err == nil {
					agentId := m.Header.Get("agentId")
					if agentId == "" {
						n.logger.Error("failed to get agentId from header")
						return true
					}

					t := new(models.AgentSummary)
					err = json.Unmarshal(m.Data, t)
					if err == nil {
						as[agentId] = *t
					}
				}
				errs = errors.Join(errs, err)
				return true
			})
		} else {
			errs = errors.Join(errs, err)
		}

		if errs != nil {
			n.logger.Error("errors in gathering agent summaries", slog.Any("errs", errs))
		}
		err = r.RespondJSON(models.NodeInfoResponse{
			AgentSummaries: as,
			NodeId:         pubKey,
			Xkey:           pubXKey,
			Tags:           n.tags,
			Uptime:         time.Since(n.startTime).String(),
			Version:        n.version,
		})
		if err != nil {
			n.logger.Error("failed to respond to node info request", slog.String("err", err.Error()))
			return
		}
	}
}

func (n *NexNode) handleAuction() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.control.namespace.AUCTION
		splitSub := strings.SplitN(r.Subject(), ".", 4)
		namespace := splitSub[2]

		req := new(models.AuctionRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal auction request")
			return
		}

		// If node doesnt have agent type, request is thrown away
		_, reg, ok := n.regs.Find(req.AgentType)
		if !ok {
			n.logger.Log(n.ctx, shandler.LevelTrace, "no valid agents found for this workload", slog.String("agent_type", req.AgentType))
			return
		}

		// If all auction tags aren't satisfied, request is thrown away
		for k, v := range req.Tags {
			if tV, ok := n.tags[k]; !ok || tV != v {
				n.logger.Log(n.ctx, shandler.LevelTrace, "workload tag not satisfied during auction", slog.String("tag", k), slog.String("value", v))
				return
			}
		}

		if n.auctioneer != nil {
			err = n.auctioneer.Auction(namespace, req.AgentType, req.Tags)
			if err != nil {
				n.logger.Error("auctioneer failed to pass auction", slog.String("err", err.Error()))
				return
			}
		}

		bidderId := n.idgen.Generate(nil)
		n.auctionMap.Put(bidderId, "", nil)

		n.logger.Debug("responding to auction", slog.Any("auctionId", req.AuctionId))
		err = r.RespondJSON(models.AuctionResponse{
			BidderId:            bidderId,
			Xkey:                reg.OriginalRequest.PublicXkey,
			StartRequestSchema:  reg.OriginalRequest.StartRequestSchema,
			SupportedLifecycles: reg.OriginalRequest.SupportedLifecycles,
		})
		if err != nil {
			n.logger.Error("failed to respond to auction request", slog.String("err", err.Error()))
			return
		}
	}
}

func (n *NexNode) handleAuctionDeployWorkload() func(micro.Request) {
	return func(r micro.Request) {
		splitSub := strings.SplitN(r.Subject(), ".", 6)
		namespace := splitSub[2]
		bidId := splitSub[5]

		if !n.auctionMap.Exists(bidId) {
			// not this nodes bidder id (or it expired), throw away request
			return
		}

		req := new(models.StartWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal auction deploy workload request")
			return
		}

		if namespace != req.Namespace && namespace != models.SystemNamespace {
			n.handlerError(r, errors.New("namespace mismatch"), "100", fmt.Sprintf("namespace mismatch: %s != %s", namespace, req.Namespace))
			return
		}

		aid, reg, ok := n.regs.Find(req.WorkloadType)
		if !ok {
			n.handlerError(r, errors.New("workload type not found"), "100", "workload type not found")
			return
		}

		rr, err := jsonschema.UnmarshalJSON(strings.NewReader(req.RunRequest))
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal start request")
			return
		}

		err = reg.Schema.Validate(rr)
		if err != nil {
			n.handlerError(r, err, "100", "failed to validate run request")
			return
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, "100", "failed to get public key from keypair")
			return
		}

		workloadId := n.idgen.Generate(req)
		wlNatsConn, err := n.minter.Mint(models.WorkloadCred, namespace, workloadId)
		if err != nil {
			n.handlerError(r, err, "100", "failed to mint workload nats connection")
			return
		}

		aReq := new(models.AgentStartWorkloadRequest)
		aReq.Request = *req
		aReq.WorkloadCreds = *wlNatsConn

		aReqB, err := json.Marshal(aReq)
		if err != nil {
			n.handlerError(r, err, "100", "failed to marshal agent start workload request")
			return
		}

		auctionDeploy, err := n.nc.Request(models.AgentAPIStartWorkloadRequestSubject(pubKey, aid, workloadId), aReqB, time.Minute)
		if err != nil {
			n.handlerError(r, err, "100", "failed to publish start workload request")
			return
		}

		err = r.Respond(auctionDeploy.Data, micro.WithHeaders(micro.Headers(auctionDeploy.Header)))
		if err != nil {
			n.logger.Error("failed to respond to auction deploy workload request", slog.String("err", err.Error()))
			return
		}

		err = n.state.StoreWorkload(workloadId, *req)
		if err != nil {
			n.logger.Warn("failed to store node state", slog.String("err", err.Error()))
			return
		}
	}
}

func (n *NexNode) handleStopWorkload() func(micro.Request) {
	return func(r micro.Request) {
		splitSub := strings.SplitN(r.Subject(), ".", 6)
		namespace := splitSub[2]
		workloadId := splitSub[5]

		req := new(models.StopWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal stop workload request")
			return
		}

		if namespace != req.Namespace && namespace != models.SystemNamespace {
			n.handlerError(r, errors.New("namespace mismatch"), "100", fmt.Sprintf("namespace mismatch: %s != %s", namespace, req.Namespace))
			return
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, "100", "failed to get public key from keypair")
			return
		}

		ret := &models.StopWorkloadResponse{
			Id:           workloadId,
			Message:      string(models.GenericErrorsWorkloadNotFound),
			Stopped:      false,
			WorkloadType: "",
		}

		msgs, err := natsext.RequestMany(n.ctx, n.nc, models.AgentAPIStopWorkloadRequestSubject(pubKey, workloadId), r.Data(), natsext.RequestManyMaxMessages(n.regs.Count()))
		if err != nil {
			err = r.RespondJSON(ret)
			if err != nil {
				n.logger.Error("failed to respond to stop workload request", slog.String("err", err.Error()))
				return
			}
		}

		msgs(func(m *nats.Msg, e error) bool {
			if e == nil && m.Data != nil && string(m.Data) != "null" {
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

		err = r.RespondJSON(ret)
		if err != nil {
			n.logger.Error("failed to respond to stop workload request", slog.String("err", err.Error()))
			return
		}

		err = n.state.RemoveWorkload(ret.WorkloadType, workloadId)
		if err != nil {
			n.logger.Warn("failed to delete node state", slog.String("err", err.Error()))
			return
		}
	}
}

func (n *NexNode) handleCloneWorkload() func(micro.Request) {
	return func(r micro.Request) {
		splitSub := strings.SplitN(r.Subject(), ".", 6)
		namespace := splitSub[2]
		workloadId := splitSub[5]

		req := new(models.CloneWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal clone workload request")
			return
		}

		if namespace != req.Namespace && namespace != models.SystemNamespace {
			n.handlerError(r, errors.New("namespace mismatch"), "100", fmt.Sprintf("namespace mismatch: %s != %s", namespace, req.Namespace))
			return
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, "100", "failed to get public key from keypair")
			return
		}

		getWorkload, err := n.nc.Request(models.AgentAPIGetWorkloadRequestSubject(pubKey, workloadId), r.Data(), time.Second*3)
		if err != nil {
			n.logger.Debug("failed to find workload request", slog.String("err", err.Error()))
			return
		}

		if getWorkload.Header.Get("Nats-Service-Error") == string(models.GenericErrorsWorkloadNotFound) {
			return
		}

		err = r.Respond(getWorkload.Data)
		if err != nil {
			n.logger.Error("failed to respond to clone workload request", slog.String("err", err.Error()))
			return
		}
	}
}

func (n *NexNode) handleNamespacePing() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.control.namespace.WPING
		splitSub := strings.SplitN(r.Subject(), ".", 4)
		namespace := splitSub[2]

		req := new(models.AgentListWorkloadsRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal ping request")
			return
		}

		if namespace != req.Namespace && namespace != models.SystemNamespace {
			n.handlerError(r, errors.New("namespace mismatch"), "100", fmt.Sprintf("namespace mismatch: %s != %s", namespace, req.Namespace))
			return
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, "100", "failed to get public key from keypair")
			return
		}

		resp := models.AgentListWorkloadsResponse{}
		msgs, err := natsext.RequestMany(n.ctx, n.nc, models.AgentAPIQueryWorkloadsSubject(pubKey), r.Data(), natsext.RequestManyMaxMessages(n.regs.Count()))
		if err != nil {
			respB, err := json.Marshal(resp)
			if err != nil {
				n.handlerError(r, err, "100", "failed to marshal response")
				return
			}
			err = r.Error("100", "failed to publish query workloads request", respB)
			if err != nil {
				n.logger.Error("failed to send micro request error message", slog.String("err", err.Error()))
			}
			return
		}

		var errs error
		msgs(func(m *nats.Msg, err error) bool {
			if err == nil && m.Data != nil {
				tResp := models.AgentListWorkloadsResponse{}
				err = json.Unmarshal(m.Data, &tResp)
				if err == nil {
					resp = append(resp, tResp...)
				}
			}
			errs = errors.Join(errs, err)
			return true
		})

		respB, err := json.Marshal(resp)
		if err != nil {
			n.handlerError(r, err, "100", "failed to marshal response")
			return
		}

		err = r.Respond(respB)
		if err != nil {
			n.logger.Error("failed to respond to namespace ping request", slog.String("err", err.Error()))
			return
		}
	}
}

// TODO: future work
// func (n *NexNode) handleStartAgent() func(micro.Request) {
// 	return func(r micro.Request) {
// 	}
// }
//
// func (n *NexNode) handleStopAgent() func(micro.Request) {
// 	return func(r micro.Request) {
// 	}
// }

func (n *NexNode) handleRegisterAgent() func(micro.Request) {
	return func(r micro.Request) {
		splitSub := strings.SplitN(r.Subject(), ".", 6)
		agentId := splitSub[5]

		registrationRequest := new(models.RegisterAgentRequest)
		err := json.Unmarshal(r.Data(), registrationRequest)
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal register local agent request")
			return
		}

		if registrationRequest.RegisterType == "" {
			n.handlerError(r, errors.New("register_type is required"), "100", "register_type is required")
			return
		}

		err = n.aregistrar.RegisterAgent(registrationRequest)
		if err != nil {
			n.handlerError(r, err, "100", "failed agent registrar check")
			return
		}

		if !n.regs.Has(agentId) {
			n.handlerError(r, errors.New(registrationRequest.Name+" agent provided invalid agentid ["+agentId+"]"), "100", "invalid agent id provided")
			return
		}

		rawSchema, err := jsonschema.UnmarshalJSON(strings.NewReader(registrationRequest.StartRequestSchema))
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal start request schema")
			return
		}

		fileLocation := filepath.Join(os.TempDir(), fmt.Sprintf("%d.json", rand.Intn(1_000_000)))
		defer func() {
			_ = os.RemoveAll(fileLocation)
		}()

		c := jsonschema.NewCompiler()
		if err := c.AddResource(fileLocation, rawSchema); err != nil {
			n.handlerError(r, err, "100", "failed to add resource")
			return
		}

		schema, err := c.Compile(fileLocation)
		if err != nil {
			n.handlerError(r, err, "100", "failed to compile schema")
			return
		}

		nodePubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, "100", "failed to get public key from keypair")
			return
		}

		err = n.regs.Update(agentId, &models.Reg{
			OriginalRequest: registrationRequest,
			Schema:          schema,
		})
		if err != nil {
			n.handlerError(r, err, "100", "failed to update registration")
			return
		}

		natsConn, err := n.minter.Mint(models.AgentCred, "", agentId)
		if err != nil {
			n.handlerError(r, err, "100", "failed to mint nats connection")
			return
		}

		agentState, err := n.state.GetStateByAgent(registrationRequest.RegisterType)
		if err != nil {
			n.logger.Warn("failed to get agent state", slog.String("err", err.Error()))
		}

		state := models.RegisterAgentResponseExistingState{}
		for workloadId, swr := range agentState {
			natsConn, err := n.minter.Mint(models.WorkloadCred, swr.Namespace, workloadId)
			if err != nil {
				n.logger.Warn("failed to mint workload nats connection", slog.String("err", err.Error()), slog.String("namespace", swr.Namespace), slog.String("workload_id", workloadId))
				continue
			}
			aswr := models.AgentStartWorkloadRequest{
				Request:       swr,
				WorkloadCreds: *natsConn,
			}
			state[workloadId] = aswr
		}

		err = r.RespondJSON(models.RegisterAgentResponse{
			ConnectionData: *natsConn,
			NodeId:         nodePubKey,
			Success:        true,
			ExistingState:  state,
		})
		if err != nil {
			n.logger.Error("failed to respond to register local agent request", slog.String("err", err.Error()))
			return
		}
		n.logger.Info("agent registered", slog.String("name", registrationRequest.Name), slog.String("type", registrationRequest.RegisterType), slog.String("agent_id", agentId))
	}
}

func (n *NexNode) handleInitRegisterRemoteAgent() func(micro.Request) {
	return func(r micro.Request) {
		req := new(models.RegisterRemoteAgentRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal register remote agent request")
			return
		}

		err = n.aregistrar.RegisterRemoteInit(req)
		if err != nil {
			n.handlerError(r, err, "100", "failed agent registrar check")
			return
		}

		agentId := n.idgen.Generate(nil)
		err = n.regs.New(agentId, models.ReqTypeRemoteAgent, req.PublicSigningKey)
		if err != nil {
			n.handlerError(r, err, "100", "failed to register remote agent")
			return
		}

		pubNodeKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, "100", "failed to get public key from keypair")
			return
		}

		connData, err := n.minter.MintRegister(agentId, pubNodeKey)
		if err != nil {
			n.handlerError(r, err, "100", "failed to mint register")
			return
		}

		ret := models.RegisterRemoteAgentResponse{
			AssignedAgentId:   agentId,
			RegistrationCreds: connData,
			RespondTo:         pubNodeKey,
		}

		err = r.RespondJSON(ret)
		if err != nil {
			n.logger.Error("failed to respond to register remote agent request", slog.String("err", err.Error()))
			return
		}
	}
}

func (n *NexNode) handleGetAgentIdByName() func(micro.Request) {
	return func(r micro.Request) {
		agentName := string(r.Data())
		if agentName == "" {
			_ = r.Respond([]byte{})
			return
		}
		agentId, _, found := n.regs.Find(agentName)
		if !found {
			_ = r.Respond([]byte{})
			return
		}
		err := r.Respond([]byte(agentId))
		if err != nil {
			n.logger.Error("failed to respond to get agent id by name request", slog.String("err", err.Error()))
			return
		}
	}
}

func (n *NexNode) handlerError(r micro.Request, err error, code, msg string) {
	if msg != "" {
		n.logger.Error(msg, slog.String("err", err.Error()))
	}

	errMsg := struct {
		Error string `json:"error"`
	}{
		Error: err.Error(),
	}

	errMsgB, err := json.Marshal(errMsg)
	if err != nil {
		n.logger.Error("failed to marshal error message", slog.String("err", err.Error()))
		errMsgB = []byte(`{}`)
	}

	// TODO: look into how to preserve original error as output by the logger
	err = r.Error(code, msg, errMsgB)
	if err != nil {
		n.logger.Error("failed to send micro request error message", slog.String("err", err.Error()))
	}
}
