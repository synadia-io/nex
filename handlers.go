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
			State:      n.state,
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

		inbox := nats.NewInbox()
		go func() {
			sub, err := n.nc.Subscribe(inbox, func(m *nats.Msg) {
				var ldr models.LameduckResponse

				agentId := m.Header.Get("agentId")
				if agentId == "" {
					n.logger.Error("failed to get agentId from header")
					return
				}

				err := json.Unmarshal(m.Data, &ldr)
				if err != nil {
					n.logger.Error("failed to unmarshal lame duck response", slog.Any("err", err), slog.String("data", string(m.Data)))
					return
				}

				err = emitSystemEvent(n.nc, n.nodeKeypair, &models.AgentLameduckSetEvent{
					Success: ldr.Success,
				})
				if err != nil {
					n.logger.Error("failed to emit system event", slog.Any("err", err))
				}
			})
			if err != nil {
				n.handlerError(r, err, "100", "failed to subscribe to inbox")
				return
			}
			defer func() {
				err := sub.Unsubscribe()
				if err != nil {
					n.logger.Error("failed to unsubscribe from inbox", slog.Any("err", err))
				}
			}()
		}()

		err = n.nc.PublishRequest(models.AgentAPISetLameduckSubject(pubKey), inbox, ldReqB)
		if err != nil {
			n.handlerError(r, err, "100", "failed to publish lameduck request")
			return
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

		as := models.AgentSummaries{}
		inbox := nats.NewInbox()
		sub, err := n.nc.Subscribe(inbox, func(m *nats.Msg) {
			var agent models.AgentSummary

			agentId := m.Header.Get("agentId")
			if agentId == "" {
				n.logger.Error("failed to get agentId from header")
				return
			}

			err := json.Unmarshal(m.Data, &agent)
			if err != nil {
				n.logger.Error("failed to unmarshal agent summary", slog.Any("err", err), slog.String("data", string(m.Data)))
				return
			}
			as[agentId] = agent
		})
		if err != nil {
			n.handlerError(r, err, "100", "failed to subscribe to inbox")
			return
		}
		defer func() {
			err := sub.Unsubscribe()
			if err != nil {
				n.logger.Error("failed to unsubscribe from inbox", slog.Any("err", err))
			}
		}()

		ctx, cancel := context.WithTimeout(n.ctx, time.Second*5)
		go func() {
			for range time.Tick(250 * time.Millisecond) {
				select {
				case <-ctx.Done():
					return
				default:
					if len(as) == n.agentCount() {
						cancel()
					}
				}
			}
		}()

		err = n.nc.PublishRequest(models.AgentAPIPingAllSubject(pubKey), inbox, nil)
		if err != nil {
			n.handlerError(r, err, "100", "failed to publish ping agents request")
			return
		}

		<-ctx.Done()

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

		// If node doesnt have agent type, request is thrown away
		_, reg, ok := n.regs.Find(req.AgentType)
		if !ok {
			return
		}

		// If all auction tags aren't satisfied, request is thrown away
		for k, v := range req.Tags {
			if tV, ok := n.tags[k]; !ok || tV != v {
				return
			}
		}

		bidderId := nuid.New().Next()
		n.auctionMap.Put(bidderId, "", nil)

		n.logger.Debug("responding to auction", slog.Any("auctionId", req.AuctionId))
		err = r.RespondJSON(models.AuctionResponse{
			BidderId:            bidderId,
			Xkey:                reg.OriginalRequest.PublicXkey,
			StartRequestSchema:  reg.OriginalRequest.StartRequestSchema,
			SupportedLifecycles: reg.OriginalRequest.SupportedLifecycles,
		})
		if err != nil {
			n.logger.Error("failed to respond to auction request", slog.Any("err", err))
			return
		}
	}
}

func (n *NexNode) handleDirectDeploy() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.control.system.DDEPLOY.NCYUEPWG2LI6EHCCWOP4UR7R7EH2KWZ3HOVQIVHL6COEX3LZ4NECXAXG
		splitSub := strings.SplitN(r.Subject(), ".", 5)
		namespace := splitSub[2]

		if namespace != models.SystemNamespace {
			n.handlerError(r, errors.New("namespace mismatch"), "100", fmt.Sprintf("namespace mismatch: %s != %s", namespace, models.SystemNamespace))
			return
		}

		req := new(models.StartWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal auction deploy workload request")
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
		err = n.nc.PublishRequest(models.AgentAPIStartWorkloadRequestSubject(pubKey, aid, workloadId), r.Reply(), r.Data())
		if err != nil {
			n.handlerError(r, err, "100", "failed to publish start workload request")
			return
		}

		if !n.noState {
			kv, err := n.jsCtx.KeyValue(n.ctx, nodeStateBucket)
			if err != nil {
				n.logger.Warn("failed to get node state for creation", slog.Any("err", err))
				return
			}
			_, err = kv.Create(n.ctx, fmt.Sprintf("%s_%s", req.WorkloadType, workloadId), r.Data())
			if err != nil {
				n.logger.Warn("failed to store node state", slog.Any("err", err))
				return
			}
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

		auctionDeploy, err := n.nc.Request(models.AgentAPIStartWorkloadRequestSubject(pubKey, aid, workloadId), aReqB, time.Second*5)
		if err != nil {
			n.handlerError(r, err, "100", "failed to publish start workload request")
			return
		}

		err = r.Respond(auctionDeploy.Data)
		if err != nil {
			n.logger.Error("failed to respond to auction deploy workload request", slog.Any("err", err))
			return
		}

		if !n.noState {
			kv, err := n.jsCtx.KeyValue(n.ctx, nodeStateBucket)
			if err != nil {
				n.logger.Warn("failed to get node state for creation", slog.Any("err", err))
				return
			}
			_, err = kv.Create(n.ctx, fmt.Sprintf("%s_%s", req.WorkloadType, workloadId), r.Data())
			if err != nil {
				n.logger.Warn("failed to store node state", slog.Any("err", err))
				return
			}
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

		// err = n.nc.PublishRequest(models.AgentAPIStopWorkloadRequestSubject(pubKey, workloadId), r.Reply(), r.Data())
		// if err != nil {
		// 	n.handlerError(r, err, "100", "failed to publish stop workload request")
		// 	return
		// }

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

		if !n.noState {
			kv, err := n.jsCtx.KeyValue(n.ctx, nodeStateBucket)
			if err != nil {
				n.logger.Warn("failed to get node state for deletion", slog.Any("err", err))
				return
			}
			keyListener, err := kv.ListKeys(n.ctx)
			if err != nil {
				n.logger.Warn("failed to list keys", slog.Any("err", err))
				return
			}
			for key := range keyListener.Keys() {
				if strings.HasSuffix(key, workloadId) {
					err = kv.Delete(n.ctx, key)
					if err != nil {
						n.logger.Warn("failed to delete node state", slog.Any("err", err))
						return
					}
					break
				}
			}
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

		// err = n.nc.PublishRequest(models.AgentAPIQueryWorkloadsSubject(pubKey), r.Reply(), r.Data())
		// if err != nil {
		// 	n.handlerError(r, err, "100", "failed to publish query workloads request")
		// 	return
		// }

		queryWorkload, err := n.nc.Request(models.AgentAPIQueryWorkloadsSubject(pubKey), r.Data(), time.Second*5)
		if err != nil {
			n.handlerError(r, err, "100", "failed to publish query workloads request")
			return
		}

		err = r.Respond(queryWorkload.Data)
		if err != nil {
			n.logger.Error("failed to respond to namespace ping request", slog.Any("err", err))
			return
		}
	}
}

func (n *NexNode) handleStartAgent() func(micro.Request) {
	return func(r micro.Request) {
	}
}

func (n *NexNode) handleStopAgent() func(micro.Request) {
	return func(r micro.Request) {
	}
}

func (n *NexNode) handleRegisterLocalAgent() func(micro.Request) {
	return func(r micro.Request) {
		registrationRequest := new(models.RegisterAgentRequest)
		err := json.Unmarshal(r.Data(), registrationRequest)
		if err != nil {
			n.handlerError(r, err, "100", "failed to unmarshal register local agent request")
			return
		}

		if !n.regs.Has(registrationRequest.AssignedId) {
			n.handlerError(r, errors.New(registrationRequest.Name+" agent provided invalid agentid ["+registrationRequest.AssignedId+"]"), "100", "invalid agent id provided")
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

		err = n.regs.Update(registrationRequest.AssignedId, &models.Reg{
			OriginalRequest: registrationRequest,
			Schema:          schema,
		})
		if err != nil {
			n.handlerError(r, err, "100", "failed to update registration")
			return
		}

		natsConn, err := n.minter.Mint(models.AgentCred, "", registrationRequest.AssignedId)
		if err != nil {
			n.handlerError(r, err, "100", "failed to mint nats connection")
			return
		}

		var state models.RegisterAgentResponseExistingState
		if !n.noState {
			state = make(models.RegisterAgentResponseExistingState)
			kv, err := n.jsCtx.KeyValue(n.ctx, nodeStateBucket)
			if err != nil {
				n.logger.Warn("failed to get node state kv", slog.Any("err", err))
				return
			}
			keyListener, err := kv.ListKeys(n.ctx)
			if err != nil {
				n.logger.Warn("failed to list keys", slog.Any("err", err))
				return
			}
			for key := range keyListener.Keys() {
				if strings.HasPrefix(key, registrationRequest.Name) {
					workloadId := strings.TrimPrefix(key, registrationRequest.Name+"_")

					kve, err := kv.Get(n.ctx, key)
					if err != nil {
						n.logger.Warn("failed to get workload state", slog.String("workload_id", key), slog.Any("err", err))
						continue
					}
					sr := new(models.StartWorkloadRequest)
					err = json.Unmarshal(kve.Value(), sr)
					if err != nil {
						n.logger.Warn("failed to unmarshal workload state", slog.String("workload_id", key), slog.Any("err", err))
						continue
					}

					wlNatsConn, err := n.minter.Mint(models.WorkloadCred, sr.Namespace, workloadId)
					if err != nil {
						n.handlerError(r, err, "100", "failed to mint workload nats connection")
						return
					}

					asr := models.AgentStartWorkloadRequest{
						Request:       *sr,
						WorkloadCreds: *wlNatsConn,
					}

					state[workloadId] = asr
				}
			}
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
		n.logger.Info("agent registered", slog.String("name", registrationRequest.Name), slog.String("agent_id", registrationRequest.AssignedId))
	}
}

func (n *NexNode) handleRegisterRemoteAgent() func(micro.Request) {
	return func(r micro.Request) {
		n.handlerError(r, errors.New("not implemented"), "100", "endpoint not implemented")
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
