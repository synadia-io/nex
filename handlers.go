package nex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/synadia-io/orbit.go/natsext"
	"github.com/synadia-labs/nex/models"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"github.com/nats-io/nuid"
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
			Version:    VERSION,
			Xkey:       pubXKey,
			State:      n.nodeState,
		})
		if err != nil {
			n.logger.Error("failed to respond to node info request", slog.Any("err", err))
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

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		// TODO: Adds agentid to lameduck response
		var errs error
		msgs, err := natsext.RequestMany(ctx, n.nc, models.AgentAPISetLameduckSubject(pubKey), ldReqB, natsext.RequestManyStall(500*time.Millisecond))
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
			n.logger.Error("failed to respond to lameduck request", slog.Any("err", err))
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

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		var errs error
		as := models.AgentSummaries{}
		msgs, err := natsext.RequestMany(ctx, n.nc, models.AgentAPIPingAllSubject(pubKey), nil, natsext.RequestManyStall(500*time.Millisecond))
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
			Version:        VERSION,
		})
		if err != nil {
			n.logger.Error("failed to respond to node info request", slog.Any("err", err))
			return
		}
	}
}

func (n *NexNode) handleAuction() func(micro.Request) {
	return func(r micro.Request) {
		req := new(models.AuctionRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal auction request")
			return
		}

		aResp, err := n.auctioneer.Auction(req.AuctionId, req.AgentType, req.Tags, n.tags, n.logger)
		if err != nil {
			n.handlerError(r, err, "100", "failed to auction")
			return
		}

		err = r.RespondJSON(aResp)
		if err != nil {
			n.logger.Error("failed to respond to auction request", slog.Any("err", err))
			return
		}
	}
}

func (n *NexNode) handleAuctionDeployWorkload() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.control.namespace.ADEPLOY.bidid
		splitSub := strings.SplitN(r.Subject(), ".", 5)
		namespace := splitSub[2]
		bidId := splitSub[4]

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

		workloadId := nuid.New().Next()
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

		err = r.Respond(auctionDeploy.Data)
		if err != nil {
			n.logger.Error("failed to respond to auction deploy workload request", slog.Any("err", err))
			return
		}

		err = n.state.StoreWorkload(workloadId, *req)
		if err != nil {
			n.logger.Warn("failed to store node state", slog.Any("err", err))
			return
		}
	}
}

func (n *NexNode) handleStopWorkload() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.control.system.UNDEPLOY.m3hNxFzennTGa0PLJxNKYk
		// $NEX.control.namespace.UNDEPLOY.workloadid
		splitSub := strings.SplitN(r.Subject(), ".", 5)
		namespace := splitSub[2]
		workloadId := splitSub[4]

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

		stopWorkload, err := n.nc.Request(models.AgentAPIStopWorkloadRequestSubject(pubKey, workloadId), r.Data(), time.Second*5)
		if err != nil {
			n.handlerError(r, err, "100", "failed to publish stop workload request")
			return
		}

		err = r.Respond(stopWorkload.Data)
		if err != nil {
			n.logger.Error("failed to respond to stop workload request", slog.Any("err", err))
			return
		}

		ret := new(models.StopWorkloadResponse)
		err = json.Unmarshal(stopWorkload.Data, ret)
		if err != nil {
			n.logger.Error("failed to unmarshal stop workload response", slog.Any("err", err))
			return
		}

		err = n.state.RemoveWorkload(ret.WorkloadType, workloadId)
		if err != nil {
			n.logger.Warn("failed to delete node state", slog.Any("err", err))
			return
		}
	}
}

func (n *NexNode) handleCloneWorkload() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.control.namespace.CLONE.workloadid
		splitSub := strings.SplitN(r.Subject(), ".", 5)
		namespace := splitSub[2]
		workloadId := splitSub[4]

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
			n.logger.Debug("failed to find workload request", slog.Any("err", err))
			return
		}

		if getWorkload.Header.Get("Nats-Service-Error") == "workload not found" {
			return
		}

		err = r.Respond(getWorkload.Data)
		if err != nil {
			n.logger.Error("failed to respond to clone workload request", slog.Any("err", err))
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		msgs, err := natsext.RequestMany(ctx, n.nc, models.AgentAPIQueryWorkloadsSubject(pubKey), r.Data(), natsext.RequestManyStall(1000*time.Millisecond))
		if err != nil {
			respB, err := json.Marshal(resp)
			if err != nil {
				n.handlerError(r, err, "100", "failed to marshal response")
				return
			}
			err = r.Error("100", "failed to publish query workloads request", respB)
			if err != nil {
				n.logger.Error("failed to send micro request error message", slog.Any("err", err))
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
			n.logger.Error("failed to respond to namespace ping request", slog.Any("err", err))
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

func (n *NexNode) handleRegisterLocalAgent() func(micro.Request) {
	return func(r micro.Request) {
		// return fmt.Sprintf("%s.%s.REGISTER.%s", AgentAPIPrefix, inAgentId, inNodeId)
		splitSub := strings.SplitN(r.Subject(), ".", 5)
		agentId := splitSub[2]

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
		defer os.RemoveAll(fileLocation)

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
			n.logger.Warn("failed to get agent state", slog.Any("err", err))
		}

		state := models.RegisterAgentResponseExistingState{}
		for workloadId, swr := range agentState {
			natsConn, err := n.minter.Mint(models.WorkloadCred, swr.Namespace, workloadId)
			if err != nil {
				n.logger.Warn("failed to mint workload nats connection", slog.Any("err", err), slog.String("namespace", swr.Namespace), slog.String("workload_id", workloadId))
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
			n.logger.Error("failed to respond to register local agent request", slog.Any("err", err))
			return
		}
		n.logger.Info("agent registered", slog.String("name", registrationRequest.Name), slog.String("type", registrationRequest.RegisterType), slog.String("agent_id", agentId))
	}
}

func (n *NexNode) handleRegisterRemoteAgent() func(micro.Request) {
	return func(r micro.Request) {
		req := new(models.RegisterRemoteAgentRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal register remote agent request")
			return
		}

		agentId := nuid.New().Next()
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
			n.logger.Error("failed to respond to register remote agent request", slog.Any("err", err))
			return
		}
	}
}

func (n *NexNode) handlerError(r micro.Request, err error, code, msg string) {
	if msg != "" {
		n.logger.Error(msg, slog.Any("err", err))
	}
	err = r.Error(code, msg, []byte(err.Error()))
	if err != nil {
		n.logger.Error("failed to send micro request error message", slog.Any("err", err))
	}
}
