package nex

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"disorder.dev/shandler"
	"github.com/synadia-io/nex/internal"
	"github.com/synadia-io/nex/models"
	"github.com/synadia-io/orbit.go/natsext"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func (n *NexNode) handlePlacementTagPing() func(micro.Request) {
	return func(r micro.Request) {
		err := r.RespondJSON(n.tags)
		if err != nil {
			n.logger.Error("failed to respond to placement tag ping request", slog.String("err", err.Error()))
			return
		}
	}
}

func (n *NexNode) handlePing() func(micro.Request) {
	return func(r micro.Request) {
		rep := new(models.NodePingRequest)
		err := json.Unmarshal(r.Data(), rep)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to unmarshal ping request")
			return
		}

		for k, v := range rep.Filter {
			if tV, ok := n.tags[k]; !ok || tV != v {
				return
			}
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to get public key from keypair")
			return
		}

		pubXKey, err := n.nodeXKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to get public xkey from xkeypair")
			return
		}

		err = r.RespondJSON(models.NodePingResponse{
			AgentCount: n.registeredAgents.Count(),
			NodeId:     pubKey,
			Tags:       n.tags,
			StartTime:  n.startTime,
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
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to unmarshal lameduck request")
			return
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to get public key from keypair")
			return
		}

		if req.Tag != nil {
			for k, v := range req.Tag {
				if tV, ok := n.tags[k]; !ok || tV != v {
					n.logger.Debug("workload tag not satisfied during lameduck", slog.String("node_id", pubKey), slog.String("tag", k), slog.String("value", v))
					return
				}
			}
		}

		delay, err := time.ParseDuration(req.Delay)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to parse lameduck delay")
			return
		}

		ldReq := models.LameduckRequest{
			Delay: delay.String(),
			Tag:   req.Tag,
		}

		ldReqB, err := json.Marshal(ldReq)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to marshal lameduck request")
			return
		}

		// TODO: Adds agentid to lameduck response
		var errs error
		msgs, err := natsext.RequestMany(n.ctx, n.nc, models.AgentAPISetLameduckSubject(pubKey), ldReqB, natsext.RequestManyMaxMessages(n.registeredAgents.Count()))
		if err == nil && msgs != nil {
			msgs(func(m *nats.Msg, err error) bool {
				if err == nil {
					agentID := m.Header.Get("agentId")
					if agentID == "" {
						errs = errors.Join(errs, errors.New("failed to get agentId from header"))
						return true
					}

					t := new(models.LameduckResponse)
					err = json.Unmarshal(m.Data, t)
					if err == nil {
						errs = errors.Join(errs, err)
						return true
					}

					err = n.eventEmitter.EmitEvent(n.id, models.AgentLameduckSetEvent{
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
			Message: fmt.Sprintf("node entering lameduck mode, will shutdown at %s", time.Now().Add(delay).Format(time.DateTime)),
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
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to get public key from keypair")
			return
		}

		pubXKey, err := n.nodeXKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to get public xkey from xkeypair")
			return
		}

		err = r.RespondJSON(models.NodeInfoResponse{
			NodeAgentSummaries: n.registeredAgents.AgentSummaries(),
			NodeId:             pubKey,
			Xkey:               pubXKey,
			Tags:               n.tags,
			Uptime:             time.Since(n.startTime).String(),
			Version:            n.version,
		})
		if err != nil {
			n.logger.Error("failed to respond to node info request", slog.String("err", err.Error()))
			return
		}
	}
}

func (n *NexNode) handleAuction() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.SVC.<namespace>.control.AUCTION
		splitSub := strings.SplitN(r.Subject(), ".", 5)
		namespace := splitSub[2]

		req := new(models.AuctionRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to unmarshal auction request")
			return
		}

		// If node doesnt have agent type, request is thrown away
		reg, err := n.registeredAgents.GetByRegisterType(req.AgentType)
		if err != nil {
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
			instaFail, err := n.auctioneer.Auction(namespace, req.AgentType, req.Tags)
			if instaFail && err != nil {
				n.handlerError(r, err, models.ErrCodeBadRequest, "failed to auction")
				return
			} else if !instaFail && err != nil {
				n.logger.Info("auctioneer failed to pass auction", slog.String("err", err.Error()))
				return
			}
		}

		bidderID := n.idgen.Generate(nil)
		n.auctionMap.Put(bidderID, "", nil)

		n.logger.Debug("responding to auction", slog.Any("auctionId", req.AuctionId))
		err = r.RespondJSON(models.AuctionResponse{
			BidderId:            bidderID,
			Xkey:                reg.RegisterRequest.PublicXkey,
			StartRequestSchema:  reg.RegisterRequest.StartRequestSchema,
			SupportedLifecycles: reg.RegisterRequest.SupportedLifecycles,
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
		bidID := splitSub[5]

		if !n.auctionMap.Exists(bidID) {
			// not this nodes bidder id (or it expired), throw away request
			return
		}

		req := new(models.StartWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to unmarshal auction deploy workload request")
			return
		}

		if namespace != req.Namespace && namespace != models.SystemNamespace {
			n.handlerError(r, errors.New("namespace mismatch"), models.ErrCodeForbidden, fmt.Sprintf("namespace mismatch: %s != %s", namespace, req.Namespace))
			return
		}

		reg, err := n.registeredAgents.GetByRegisterType(req.WorkloadType)
		if err != nil {
			n.handlerError(r, errors.New("workload type not found"), models.ErrCodeNotFound, "workload type not found")
			return
		}

		rr, err := jsonschema.UnmarshalJSON(strings.NewReader(req.RunRequest))
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to unmarshal start request")
			return
		}

		err = reg.Schema.Validate(rr)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to validate run request")
			return
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to get public key from keypair")
			return
		}

		workloadID := n.idgen.Generate(req)
		// Mint against the workload's owning namespace (from the request
		// body), not the subject namespace. The subject namespace is only
		// used for the authorization bypass check above; the workload's
		// NATS permissions — specifically the log sub scope
		// $NEX.FEED.<ns>.logs.> — must line up with where the workload
		// actually lives, which is req.Namespace.
		wlNatsConn, err := n.handlerMinter.Mint(models.WorkloadCred, req.Namespace, workloadID)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to mint workload nats connection")
			return
		}

		// Capture the minted workload's PUBLIC user nkey into the stored
		// request's metadata. This is not a secret (the seed is, and the
		// seed is never persisted here or elsewhere) -- it is the
		// prerequisite for future credential fencing (revocation): without
		// it, nobody knows which nkey to revoke for a given workload.
		// "nex_minted_nkey" is a cross-repo contract key name; do not rename.
		if req.Metadata == nil {
			req.Metadata = models.StartWorkloadRequestMetadata{}
		}
		req.Metadata["nex_minted_nkey"] = wlNatsConn.NatsUserNkey

		aReq := new(models.AgentStartWorkloadRequest)
		aReq.Request = *req
		aReq.WorkloadCreds = *wlNatsConn

		aReqB, err := json.Marshal(aReq)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to marshal agent start workload request")
			return
		}

		auctionDeploy, err := n.nc.Request(models.AgentAPIStartWorkloadRequestSubject(pubKey, reg.ID, workloadID), aReqB, time.Minute)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to publish start workload request")
			return
		}

		err = r.Respond(auctionDeploy.Data, micro.WithHeaders(micro.Headers(auctionDeploy.Header)))
		if err != nil {
			n.logger.Error("failed to respond to auction deploy workload request", slog.String("err", err.Error()))
			return
		}

		// CREATE-ONLY (expectedRevision 0). This store runs AFTER the
		// caller already has its workload id back, so a caller can issue an
		// UPDATE against that id and have it land — store-first and all —
		// before this line executes. A blind write here would then revert
		// that update with no signal to anyone, which is precisely what
		// blocks a control plane from doing deploy-then-update sequences.
		// Creating rather than putting makes the loss impossible: whoever
		// occupied the key did so with a definition newer than this one, so
		// the newer definition is the one that stays.
		//
		// The response to the caller was already sent above, so a store
		// failure here does not fail the deploy — but it means the
		// workload's minted nkey (including the metadata this handler just
		// stamped) is never persisted at all: credential fencing against
		// this workload later would have no record to revoke against.
		err = n.state.StoreWorkload(workloadID, *req, 0)
		if errors.Is(err, models.ErrStateConflict) {
			n.logger.Warn("workload record already exists (concurrent update) — leaving the newer definition", slog.String("workload_id", workloadID))
			return
		}
		if err != nil {
			n.logger.Error("failed to persist minted workload nkey; workload record missing — credential fencing against this workload has nothing to revoke", slog.String("err", err.Error()), slog.String("workload_id", workloadID))
			return
		}
	}
}

func (n *NexNode) handleStopWorkload() func(micro.Request) {
	return func(r micro.Request) {
		splitSub := strings.SplitN(r.Subject(), ".", 6)
		namespace := splitSub[2]
		workloadID := splitSub[5]

		req := new(models.StopWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to unmarshal stop workload request")
			return
		}

		if namespace != req.Namespace && namespace != models.SystemNamespace {
			n.handlerError(r, errors.New("namespace mismatch"), models.ErrCodeForbidden, fmt.Sprintf("namespace mismatch: %s != %s", namespace, req.Namespace))
			return
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to get public key from keypair")
			return
		}

		ret := &models.StopWorkloadResponse{
			Id:           workloadID,
			Message:      string(models.GenericErrorsWorkloadNotFound),
			Stopped:      false,
			WorkloadType: "",
		}

		msgs, err := natsext.RequestMany(n.ctx, n.nc, models.AgentAPIStopWorkloadRequestSubject(pubKey, workloadID), r.Data(), natsext.RequestManyMaxMessages(n.registeredAgents.Count()))
		if err != nil {
			err = r.RespondJSON(ret)
			if err != nil {
				n.logger.Error("failed to respond to stop workload request", slog.String("err", err.Error()))
			}
			return
		}

		if msgs != nil {
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
		}

		// Delete state before replying: the reply is the caller's signal
		// that the stop is durable, so the record must already be gone
		// when it lands.
		if ret.Stopped {
			if err := n.state.RemoveWorkload(ret.WorkloadType, workloadID); err != nil {
				n.logger.Warn("failed to delete node state", slog.String("err", err.Error()))
			}
		}

		err = r.RespondJSON(ret)
		if err != nil {
			n.logger.Error("failed to respond to stop workload request", slog.String("err", err.Error()))
			return
		}
	}
}

func (n *NexNode) handleCloneWorkload() func(micro.Request) {
	return func(r micro.Request) {
		splitSub := strings.SplitN(r.Subject(), ".", 6)
		namespace := splitSub[2]
		workloadID := splitSub[5]

		req := new(models.CloneWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to unmarshal clone workload request")
			return
		}

		if namespace != req.Namespace && namespace != models.SystemNamespace {
			n.handlerError(r, errors.New("namespace mismatch"), models.ErrCodeForbidden, fmt.Sprintf("namespace mismatch: %s != %s", namespace, req.Namespace))
			return
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to get public key from keypair")
			return
		}

		getWorkload, err := n.nc.Request(models.AgentAPIGetWorkloadRequestSubject(pubKey, workloadID), r.Data(), time.Second*3)
		if err != nil {
			n.logger.Debug("failed to find workload request", slog.String("err", err.Error()))
			return
		}

		if getWorkload.Header.Get("Nats-Service-Error") == string(models.GenericErrorsWorkloadNotFound) {
			return
		}

		// The agent's state.Exists iterates every namespace when looking
		// up a workload by id, so a user caller could otherwise fetch the
		// full StartWorkloadRequest definition (including RunRequest) of a
		// workload owned by another namespace. Verify the returned
		// workload's namespace matches the caller's, unless the caller is
		// system (which is permitted to clone across namespaces). Silent
		// drop on mismatch matches the existing "not found" silence
		// pattern above so existence is not leaked.
		tmp := new(models.StartWorkloadRequest)
		if err := json.Unmarshal(getWorkload.Data, tmp); err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to unmarshal workload definition from agent")
			return
		}
		if tmp.Namespace != namespace && namespace != models.SystemNamespace {
			return
		}

		err = r.Respond(getWorkload.Data)
		if err != nil {
			n.logger.Error("failed to respond to clone workload request", slog.String("err", err.Error()))
			return
		}
	}
}

// updateStopUnconfirmedMessage is the client-visible outcome when the
// running instance could not be confirmed stopped. It is not a failure of
// the stored definition: that has already been persisted (store-first), so
// the next agent registration starts the NEW definition and the update
// completes late rather than reverting. The wording is part of the client
// contract -- callers key remediation off it.
const updateStopUnconfirmedMessage = "stop unconfirmed; stored definition will apply on next agent registration"

// updateConcurrentModificationMessage is the client-visible outcome when the
// store-first compare-and-swap lost to another writer: the record changed
// between this handler reading it and writing it back.
//
// Unlike updateStopUnconfirmedMessage this one is NOT self-healing, and the
// difference is the point of separate wording. Nothing was persisted,
// stopped or started -- the abort lands before the stop -- so the workload
// is exactly as it was, and the definition now on file is somebody else's.
// The caller has to look at the current record and decide whether its
// change still applies, which is why the message says retry rather than
// promising a later repair. The wording is part of the client contract.
const updateConcurrentModificationMessage = "workload record was modified concurrently; retry"

// replaceError carries the micro error code and the operator-facing message
// alongside a failure, so replaceWorkload can report one without holding a
// micro.Request and its caller can pass all three to handlerError.
type replaceError struct {
	code string
	msg  string
	err  error
}

func (e *replaceError) Error() string { return e.err.Error() }

func newReplaceError(code, msg string, err error) *replaceError {
	return &replaceError{code: code, msg: msg, err: err}
}

// handleUpdateWorkload replaces the definition of an existing workload in
// place, keeping the same workload id.
//
// Every node sees every control message (the micro queue group is the
// node's own id), so this handler first has to establish that a locally
// registered nexlet actually holds the addressed workload. When it does
// not -- no nexlet has the id, or one has it under a different namespace --
// the handler drops the request silently, which is handleCloneWorkload's
// convention: answering would confirm the id's existence to a caller with
// no right to it, and answering in one case but not the other would leak
// the same thing through the reply count.
func (n *NexNode) handleUpdateWorkload() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.SVC.<namespace>.control.UPDATE.<workloadId>
		splitSub := strings.SplitN(r.Subject(), ".", 6)
		namespace := splitSub[2]
		workloadID := splitSub[5]

		req := new(models.UpdateWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to unmarshal update workload request")
			return
		}

		if namespace != req.Namespace && namespace != models.SystemNamespace {
			n.handlerError(r, errors.New("namespace mismatch"), models.ErrCodeForbidden, fmt.Sprintf("namespace mismatch: %s != %s", namespace, req.Namespace))
			return
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to get public key from keypair")
			return
		}

		// A nexlet that does not hold the id does not reply at all
		// (sdk/go/agent/runner.go handleGetWorkload), so "no reply" is the
		// not-found signal here, not an error.
		//
		// Drop silently rather than answering. Every node sees every
		// control message, so WHICH nodes answer is itself observable to
		// the caller. If an unknown id answered while an id owned by
		// another namespace stayed silent (the check below), the reply
		// count alone would separate "no such workload" from "a workload
		// you may not see" -- an existence oracle assembled out of the
		// verb's own error handling. Both cases are therefore silent,
		// matching handleCloneWorkload; a caller reads no-responders or a
		// request timeout as not-found, exactly as it already must for
		// CLONE.
		getWorkload, err := n.nc.Request(models.AgentAPIGetWorkloadRequestSubject(pubKey, workloadID), r.Data(), time.Second*3)
		if err != nil {
			n.logger.Debug("no local agent holds the workload to update", slog.String("workload_id", workloadID), slog.String("err", err.Error()))
			return
		}

		if getWorkload.Header.Get("Nats-Service-Error") == string(models.GenericErrorsWorkloadNotFound) {
			return
		}

		current := new(models.StartWorkloadRequest)
		if err := json.Unmarshal(getWorkload.Data, current); err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to unmarshal workload definition from agent")
			return
		}

		// The nexlet's lookup by id spans every namespace, so without this
		// check a caller could replace the definition of a workload owned
		// by another namespace. Silent drop, per the CLONE convention.
		if current.Namespace != namespace && namespace != models.SystemNamespace {
			return
		}

		// UPDATE replaces a definition in place; it does not relocate the
		// workload. A replacement naming a different namespace would
		// re-scope the credentials minted for it (log and trigger subjects
		// are namespace-scoped) while the workload keeps running here under
		// the old namespace's placement. System callers are not exempt:
		// relocation is not this verb's job.
		if req.StartRequest.Namespace != current.Namespace {
			n.handlerError(r, errors.New("namespace mismatch"), models.ErrCodeForbidden, fmt.Sprintf("update cannot move a workload between namespaces: %s != %s", req.StartRequest.Namespace, current.Namespace))
			return
		}

		updated, message, rerr := n.replaceWorkload(workloadID, *current, req.StartRequest)
		if rerr != nil {
			n.handlerError(r, rerr.err, rerr.code, rerr.msg)
			return
		}

		n.respondUpdateWorkload(r, models.UpdateWorkloadResponse{
			Id:      workloadID,
			Updated: updated,
			Message: message,
		})
	}
}

func (n *NexNode) respondUpdateWorkload(r micro.Request, resp models.UpdateWorkloadResponse) {
	if err := r.RespondJSON(resp); err != nil {
		n.logger.Error("failed to respond to update workload request", slog.String("err", err.Error()))
	}
}

// handleRestartWorkload restarts an existing workload from its STORED
// definition, reusing the same workload id and reusing UpdateWorkloadResponse
// as the reply type (a restart is an update whose replacement definition
// happens to be the one already on file, so no new response shape earns its
// keep).
//
// Subject parsing, namespace agreement, and the ownership-fetch/silent-drop
// convention are identical to handleUpdateWorkload -- see its comment for why
// unknown-id and not-your-namespace must be indistinguishable to the caller.
//
// The definition RESTART replays is deliberately NOT the one the ownership
// fetch just returned. That fetch reports whatever the owning nexlet
// currently has running -- "reality" -- while the node's persisted state
// record is "intent" (replaceWorkload's doc comment, and design decision D3
// in the execution plan: store-first makes the stored record the thing a
// crash-safe caller can trust). The two normally agree, but they can
// diverge exactly when a prior UPDATE didn't finish: an unconfirmed stop or
// a failed start leaves the NEW definition stored while the nexlet still
// runs (or, on a failed start, doesn't run) the OLD one. Restarting from the
// agent's live snapshot in that case would silently re-apply the stale
// definition and defeat store-first's whole point; restarting from the
// stored record instead finishes the interrupted update. current (the
// fetched definition) is used only to establish ownership and to pin the
// workload type/namespace replaceWorkload enforces.
func (n *NexNode) handleRestartWorkload() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.SVC.<namespace>.control.RESTART.<workloadId>
		splitSub := strings.SplitN(r.Subject(), ".", 6)
		namespace := splitSub[2]
		workloadID := splitSub[5]

		req := new(models.RestartWorkloadRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to unmarshal restart workload request")
			return
		}

		if namespace != req.Namespace && namespace != models.SystemNamespace {
			n.handlerError(r, errors.New("namespace mismatch"), models.ErrCodeForbidden, fmt.Sprintf("namespace mismatch: %s != %s", namespace, req.Namespace))
			return
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to get public key from keypair")
			return
		}

		// Same not-found/ownership resolution as handleUpdateWorkload: a
		// nexlet that does not hold the id does not reply at all, so "no
		// reply" is the not-found signal, and both "unknown id" and "owned
		// by another namespace" (checked below) must be silent so the reply
		// count cannot be used as an existence oracle. A caller reads
		// no-responders/timeout as not-found, exactly as it already must for
		// CLONE and UPDATE.
		getWorkload, err := n.nc.Request(models.AgentAPIGetWorkloadRequestSubject(pubKey, workloadID), r.Data(), time.Second*3)
		if err != nil {
			n.logger.Debug("no local agent holds the workload to restart", slog.String("workload_id", workloadID), slog.String("err", err.Error()))
			return
		}

		if getWorkload.Header.Get("Nats-Service-Error") == string(models.GenericErrorsWorkloadNotFound) {
			return
		}

		current := new(models.StartWorkloadRequest)
		if err := json.Unmarshal(getWorkload.Data, current); err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to unmarshal workload definition from agent")
			return
		}

		// The nexlet's lookup by id spans every namespace, so without this
		// check a caller could restart (and re-mint credentials for) a
		// workload owned by another namespace. Silent drop, per the
		// CLONE/UPDATE convention.
		if current.Namespace != namespace && namespace != models.SystemNamespace {
			return
		}

		// current only establishes ownership; the definition actually
		// replayed is the STORED one (see the function doc comment above).
		// The ownership fetch already told us the workload's type, which is
		// the other half of the record's key, so this is a direct read
		// rather than a scan of every record in the namespace.
		//
		// That scan was scoped to the workload's namespace, so the check
		// below re-applies by hand the filter it gave for free. It is
		// load-bearing, not tidiness: the key is only (type, id), and
		// replaceWorkload mints the replacement credential against the
		// namespace of the definition it is handed -- so replaying a record
		// belonging to another namespace would issue credentials scoped
		// into that namespace (log and trigger subjects are
		// namespace-scoped) and start its definition under this workload's
		// id, for a caller with every right to restart its OWN workload.
		//
		// The divergence is reachable without any out-of-band writer: the
		// workload id generator is a public node option (WithIDGenerator),
		// so a deterministic one lets two namespaces collide on one key.
		// Create-only then correctly refuses the second store, leaving one
		// namespace's record on file while the other namespace's workload
		// is the one actually running.
		storedDef, _, err := n.state.GetWorkloadRecord(current.WorkloadType, workloadID)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to read stored workload definitions")
			return
		}

		if storedDef == nil || storedDef.Namespace != current.Namespace {
			// Either nothing is on file for this workload, or what is under
			// its key belongs to another namespace and is therefore not
			// this workload's definition at all. Both mean "nothing to
			// restart from" and get the identical answer, which also keeps
			// the foreign-record case from becoming an existence oracle for
			// another tenant's data.
			//
			// The empty case is reachable in normal operation, not just as
			// a bug: handleAuctionDeployWorkload responds to the
			// deploy caller and starts the agent BEFORE calling
			// state.StoreWorkload, and does not fail the deploy if that
			// store errors -- so a workload can legitimately run with no
			// stored record at all. Ownership was already established
			// above, so there is no existence to hide by going silent here;
			// silence would also be actively misleading, since the client
			// maps no-responders to "not found" and the workload plainly
			// does exist. Report the true state instead: nothing to restart
			// from, and updated:false is truthful rather than an opaque
			// error, matching the wording style of
			// updateStopUnconfirmedMessage / replacementStartFailedMessage.
			n.respondUpdateWorkload(r, models.UpdateWorkloadResponse{
				Id:      workloadID,
				Updated: false,
				Message: "no stored definition for this workload; nothing to restart",
			})
			return
		}

		updated, message, rerr := n.replaceWorkload(workloadID, *current, *storedDef)
		if rerr != nil {
			n.handlerError(r, rerr.err, rerr.code, rerr.msg)
			return
		}

		n.respondUpdateWorkload(r, models.UpdateWorkloadResponse{
			Id:      workloadID,
			Updated: updated,
			Message: message,
		})
	}
}

// replaceWorkload swaps the definition behind workloadID for def, reusing
// the same workload id, and is the shared core of the workload-replacement
// control verbs.
//
// The step ordering is the correctness content and must not be rearranged:
//
//   - validate before persisting, so a rejected definition can never
//     displace a good stored one;
//   - mint before persisting, so the nkey in the stored record is the one
//     the replacement instance actually receives (a record holding a stale
//     nkey would make a later credential revocation revoke the wrong
//     identity);
//   - persist before stopping ("store-first"), so a crash at any point
//     after this leaves the NEW definition as the one
//     resume-on-registration will start: the repair path completes the
//     update instead of silently reverting it. That claim depends on the
//     replacement occupying the SAME state record and resuming under the
//     SAME nexlet -- which holds only because the workload type and the
//     namespace are both pinned (see the rejections below and in
//     handleUpdateWorkload). The KV key is "<workload_type>_<workload_id>"
//     (internal/state/nats_kv.go) and resume is scoped per agent type
//     (handleRegisterAgent -> GetStateByAgent), so a type change would
//     write a second key that a DIFFERENT nexlet resumes independently of
//     the one still running the old instance;
//   - start only after a CONFIRMED stop. Starting first is exactly the
//     dual-writer window this verb exists to close -- two instances of one
//     workload publishing, advancing the same checkpoints and splitting the
//     same durable consumer.
//
// current is the definition the owning nexlet holds right now. It is read
// to enforce the workload-type invariant and to address the stop at the
// workload's namespace.
//
// A false return with a nil error is a legitimate outcome rather than a
// server fault, in one of two shapes distinguished by the returned message:
//
//   - the store landed but the swap did not finish (unconfirmed stop, or a
//     failed replacement start). Self-healing: the new definition is on
//     file, and the next agent registration brings reality up to it.
//   - the store-first compare-and-swap LOST to a concurrent writer
//     (updateConcurrentModificationMessage). Nothing was persisted,
//     stopped or started -- the abort lands before the stop -- and nothing
//     will repair it later, because there is nothing half-done to repair.
//     The caller re-reads and decides.
//
// No lifecycle events are
// emitted here -- the agent's own stop and start paths emit exactly one
// WORKLOADSTOPPED and one WORKLOADSTARTED, which is what namespace quota
// accounting expects.
func (n *NexNode) replaceWorkload(workloadID string, current, def models.StartWorkloadRequest) (bool, string, *replaceError) {
	// Defensive: def.Metadata is mutated below (the nkey stamp). def and
	// current are ordinary struct values, but their Metadata fields are
	// maps -- reference types -- so if a caller ever constructs def and
	// current from the same underlying value (e.g. a same-definition
	// replacement like RESTART built the naive way, replaceWorkload(id,
	// current, current)), def.Metadata and current.Metadata would be the
	// SAME map and this function would silently mutate a map the caller
	// still holds a reference to via current. Cloning up front makes the
	// mutation-in-place below safe regardless of what the caller passed.
	def.Metadata = maps.Clone(def.Metadata)

	// A workload-type change would reopen the dual-writer window this verb
	// exists to close, and store-first makes it worse rather than better.
	// The state key embeds the type, so the replacement lands on a NEW key
	// while the old one survives; resume-on-registration is scoped per
	// agent type (handleRegisterAgent -> GetStateByAgent), so the two
	// records are resumed by two different nexlets that know nothing about
	// each other. If the stop is then unconfirmed, the old-type nexlet
	// keeps the old instance running while the new-type nexlet starts the
	// new definition: both live, which is precisely the failure mode this
	// verb removes. Purging the old key first does not fix it -- it only
	// narrows the window and adds a crash window where the workload is
	// lost entirely.
	//
	// So the type is pinned, not migrated. Rejecting is also what keeps the
	// store-first repair path sound (see the doc comment above). Enforced
	// here rather than in the caller because every caller of this function
	// depends on it, T6's RESTART included.
	if current.WorkloadType != def.WorkloadType {
		return false, "", newReplaceError(models.ErrCodeForbidden,
			"update cannot change a workload's type; undeploy and deploy instead",
			fmt.Errorf("workload type mismatch: %s != %s", def.WorkloadType, current.WorkloadType))
	}

	// The namespace is pinned for the same reason, and the doc comment
	// above depends on it being pinned HERE rather than in each caller:
	// this function mints the replacement's credential against
	// def.Namespace, and a workload credential's log and trigger subjects
	// are namespace-scoped ($NEX.FEED.<ns>.logs.>). A def naming a
	// different namespace than the running instance would therefore hand
	// the replacement authority inside a namespace the workload does not
	// live in, and address the stop at yet another one (current.Namespace).
	//
	// Both existing callers check this before they get here -- UPDATE
	// rejects a namespace change outright (handleUpdateWorkload) and
	// RESTART treats a foreign-namespace record as no record at all -- so
	// this is the invariant made unconditional rather than a new
	// restriction, and it is what any future caller inherits.
	if current.Namespace != def.Namespace {
		return false, "", newReplaceError(models.ErrCodeForbidden,
			"update cannot move a workload between namespaces; undeploy and deploy instead",
			fmt.Errorf("workload namespace mismatch: %s != %s", def.Namespace, current.Namespace))
	}

	pubKey, err := n.nodeKeypair.PublicKey()
	if err != nil {
		return false, "", newReplaceError(models.ErrCodeInternalServerError, "failed to get public key from keypair", err)
	}

	reg, err := n.registeredAgents.GetByRegisterType(def.WorkloadType)
	if err != nil {
		return false, "", newReplaceError(models.ErrCodeNotFound, "workload type not found", errors.New("workload type not found"))
	}

	rr, err := jsonschema.UnmarshalJSON(strings.NewReader(def.RunRequest))
	if err != nil {
		return false, "", newReplaceError(models.ErrCodeBadRequest, "failed to unmarshal start request", err)
	}

	if err := reg.Schema.Validate(rr); err != nil {
		return false, "", newReplaceError(models.ErrCodeBadRequest, "failed to validate run request", err)
	}

	// Credentials are handed to a workload only at create time, so a
	// replacement instance means a fresh mint regardless.
	wlNatsConn, err := n.handlerMinter.Mint(models.WorkloadCred, def.Namespace, workloadID)
	if err != nil {
		return false, "", newReplaceError(models.ErrCodeInternalServerError, "failed to mint workload nats connection", err)
	}

	// "nex_minted_nkey" is a cross-repo contract key name; do not rename.
	if def.Metadata == nil {
		def.Metadata = models.StartWorkloadRequestMetadata{}
	}
	def.Metadata["nex_minted_nkey"] = wlNatsConn.NatsUserNkey

	// STORE-FIRST, under compare-and-swap. The type is pinned above, so
	// this write lands on the same KV key the old definition occupied --
	// one record, one instance, no second key for another nexlet to resume.
	// Nothing has been stopped yet, so a failed write simply aborts the
	// update with the old instance still running.
	//
	// The read happens HERE, immediately before the write, rather than at
	// the top of the verb: everything above this point (validation, the
	// type check, the mint) can take arbitrarily long, and a revision read
	// earlier would only widen the window it is meant to close. An absent
	// record yields revision 0, which is create-only -- the right semantics
	// for the legitimate "nexlet holds it but nothing was ever persisted"
	// case (see handleRestartWorkload's stored-record branch), and still a
	// conflict if someone else creates the record first.
	_, revision, err := n.state.GetWorkloadRecord(def.WorkloadType, workloadID)
	if err != nil {
		return false, "", newReplaceError(models.ErrCodeInternalServerError, "failed to read stored workload definition", err)
	}

	// A lost CAS means someone else's definition is on file. Overwriting it
	// would be the silent revert this whole design exists to prevent, and
	// retrying here would be worse: the competing writer may itself be
	// mid-replacement (it stores before it stops), so a retry could stop an
	// instance the other writer is about to replace. Nothing has been
	// touched yet, so the honest answer is to abort and let the caller
	// decide against the newer record.
	if err := n.state.StoreWorkload(workloadID, def, revision); err != nil {
		if errors.Is(err, models.ErrStateConflict) {
			n.logger.Warn("workload update aborted: record changed concurrently", slog.String("workload_id", workloadID), slog.String("err", err.Error()))
			return false, updateConcurrentModificationMessage, nil
		}
		return false, "", newReplaceError(models.ErrCodeInternalServerError, "failed to persist updated workload definition", err)
	}

	stopReqB, err := json.Marshal(models.StopWorkloadRequest{Namespace: current.Namespace})
	if err != nil {
		return false, "", newReplaceError(models.ErrCodeInternalServerError, "failed to marshal stop workload request", err)
	}

	stopped := false
	msgs, err := natsext.RequestMany(n.ctx, n.nc, models.AgentAPIStopWorkloadRequestSubject(pubKey, workloadID), stopReqB, natsext.RequestManyMaxMessages(n.registeredAgents.Count()))
	if err == nil && msgs != nil {
		msgs(func(m *nats.Msg, e error) bool {
			if e == nil && m.Data != nil && string(m.Data) != "null" {
				swresp := new(models.StopWorkloadResponse)
				if uerr := json.Unmarshal(m.Data, swresp); uerr == nil && swresp.Stopped {
					stopped = true
					return false
				}
			}
			return true
		})
	}

	if !stopped {
		n.logger.Warn("workload update aborted before start: stop not confirmed", slog.String("workload_id", workloadID))
		return false, updateStopUnconfirmedMessage, nil
	}

	aReqB, err := json.Marshal(models.AgentStartWorkloadRequest{
		Request:       def,
		WorkloadCreds: *wlNatsConn,
	})
	if err != nil {
		return false, "", newReplaceError(models.ErrCodeInternalServerError, "failed to marshal agent start workload request", err)
	}

	// Past the confirmed stop there is nothing to roll back to: the old
	// instance is gone and the new definition is already the stored one.
	// Both ways the start can fail -- the nexlet answering with an error,
	// or not answering at all -- leave that identical state, so both get
	// the same truthful answer rather than an opaque 500 for one of them.
	// The update is not lost: resume-on-registration finishes it.
	startResp, err := n.nc.Request(models.AgentAPIStartWorkloadRequestSubject(pubKey, reg.ID, workloadID), aReqB, time.Minute)
	if err != nil {
		n.logger.Error("no reply to replacement start workload request", slog.String("workload_id", workloadID), slog.String("err", err.Error()))
		return false, replacementStartFailedMessage(err.Error()), nil
	}

	if agentErr := startResp.Header.Get("Nats-Service-Error"); agentErr != "" {
		n.logger.Error("agent failed to start replacement workload", slog.String("workload_id", workloadID), slog.String("err", agentErr))
		return false, replacementStartFailedMessage(agentErr), nil
	}

	return true, "", nil
}

// replacementStartFailedMessage is the client-visible outcome when the old
// instance was confirmed stopped but the replacement did not start. Like
// updateStopUnconfirmedMessage it reports updated:false against an ALREADY
// stored new definition, so the situation is self-healing on the next agent
// registration -- but unlike it, nothing is running in the meantime.
func replacementStartFailedMessage(cause string) string {
	return fmt.Sprintf("workload stopped but replacement failed to start (%s); stored definition will apply on next agent registration", cause)
}

func (n *NexNode) handleNamespacePing() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.control.namespace.WPING
		splitSub := strings.SplitN(r.Subject(), ".", 4)
		namespace := splitSub[2]

		req := new(models.AgentListWorkloadsRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to unmarshal ping request")
			return
		}

		if namespace != req.Namespace && namespace != models.SystemNamespace {
			n.handlerError(r, errors.New("namespace mismatch"), models.ErrCodeForbidden, fmt.Sprintf("namespace mismatch: %s != %s", namespace, req.Namespace))
			return
		}

		pubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to get public key from keypair")
			return
		}

		resp := models.AgentListWorkloadsResponse{}
		msgs, err := natsext.RequestMany(n.ctx, n.nc, models.AgentAPIQueryWorkloadsSubject(pubKey), r.Data(), natsext.RequestManyMaxMessages(n.registeredAgents.Count()))
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to publish query workloads request")
			return
		}

		var errs error
		if msgs != nil {
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
		}

		respB, err := json.Marshal(resp)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to marshal response")
			return
		}

		err = r.Respond(respB)
		if err != nil {
			n.logger.Error("failed to respond to namespace ping request", slog.String("err", err.Error()))
			return
		}
	}
}

// stampMintedNkey returns swr with the public user nkey of its currently
// minted credential recorded in metadata, which is the prerequisite for
// credential fencing: without it nobody knows which nkey to revoke for a
// given workload. The metadata map is cloned rather than mutated in place,
// so the caller's copy of swr is never altered behind its back -- the same
// precaution replaceWorkload takes, and it matters more here because the
// value being stamped is re-read from state on a retry.
//
// "nex_minted_nkey" is a cross-repo contract key name; do not rename.
func stampMintedNkey(swr models.StartWorkloadRequest, nkey string) models.StartWorkloadRequest {
	if swr.Metadata == nil {
		swr.Metadata = models.StartWorkloadRequestMetadata{}
	} else {
		swr.Metadata = maps.Clone(swr.Metadata)
	}
	swr.Metadata["nex_minted_nkey"] = nkey
	return swr
}

func (n *NexNode) handleRegisterAgent() func(micro.Request) {
	return func(r micro.Request) {
		// $NEX.SVC.<nodeid>.agent.REGISTER.<agentid>
		splitSub := strings.SplitN(r.Subject(), ".", 6)
		agentID := splitSub[5]

		registrationRequest := new(models.RegisterAgentRequest)
		err := json.Unmarshal(r.Data(), registrationRequest)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to unmarshal register local agent request")
			return
		}

		if registrationRequest.RegisterType == "" {
			n.handlerError(r, errors.New("register_type is required"), models.ErrCodeBadRequest, "register_type is required")
			return
		}

		err = n.aregistrar.RegisterAgent(r.Headers(), registrationRequest)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeForbidden, "failed agent registrar check")
			return
		}

		rawSchema, err := jsonschema.UnmarshalJSON(strings.NewReader(registrationRequest.StartRequestSchema))
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to unmarshal start request schema")
			return
		}

		fileLocation := filepath.Join(os.TempDir(), fmt.Sprintf("%d.json", rand.Intn(1_000_000)))
		defer func() {
			_ = os.RemoveAll(fileLocation)
		}()

		c := jsonschema.NewCompiler()
		if err := c.AddResource(fileLocation, rawSchema); err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to add resource")
			return
		}

		schema, err := c.Compile(fileLocation)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to compile schema")
			return
		}

		nodePubKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to get public key from keypair")
			return
		}

		p := &internal.AgentRegistration{
			ID:              agentID,
			RegisterRequest: registrationRequest,
			Schema:          schema,
		}

		err = n.registeredAgents.Add(p)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to update registration")
			return
		}

		natsConn, err := n.handlerMinter.Mint(models.AgentCred, "", agentID)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to mint nats connection")
			return
		}

		agentState, err := n.state.GetStateByAgent(registrationRequest.RegisterType)
		if err != nil {
			n.logger.Warn("failed to get agent state", slog.String("err", err.Error()))
		}

		// agentState is a SNAPSHOT, and everything below runs after it: a
		// mint per record, then a write per record. Any UPDATE that lands
		// in that window has already stored its new definition, so the
		// snapshot's copy is stale the moment it is taken. Writing the
		// snapshot back would revert that definition on disk and — because
		// the same value is what the agent is told to resume — start the
		// reverted definition too, which is the silent-revert failure the
		// store-first design exists to prevent.
		//
		// So the snapshot is used for one thing only: the list of workload
		// ids this agent type owns. Each record's CONTENT is re-read
		// immediately before it is minted and stamped.
		state := models.RegisterAgentResponseExistingState{}
		for workloadID := range agentState {
			// The key is "<workload_type>_<workload_id>" and GetStateByAgent
			// matched on exactly this prefix, so RegisterType is the type
			// half of every key it returned.
			fresh, revision, err := n.state.GetWorkloadRecord(registrationRequest.RegisterType, workloadID)
			if err != nil {
				n.logger.Error("failed to re-read workload record, workload dropped from resume state", slog.String("err", err.Error()), slog.String("workload_id", workloadID))
				continue
			}
			if fresh == nil {
				// Purged between the snapshot and now — an UNDEPLOY the
				// node confirmed. Resuming it would resurrect a workload
				// the operator stopped.
				n.logger.Warn("workload record removed during resume; not resuming", slog.String("workload_id", workloadID))
				continue
			}
			swr := *fresh

			natsConn, err := n.handlerMinter.Mint(models.WorkloadCred, swr.Namespace, workloadID)
			if err != nil {
				// Workload is dropped from the agent's resume state — surface it
				// as an error, not just a warning.
				n.logger.Error("failed to mint workload nats connection, workload dropped from resume state", slog.String("err", err.Error()), slog.String("namespace", swr.Namespace), slog.String("workload_id", workloadID))
				continue
			}

			// Creds are re-minted per record on every resume, so the
			// persisted nkey must be refreshed to track the live
			// credential — otherwise a future fencing revocation would
			// target a stale (no longer valid) nkey. Re-store the record so
			// the KV copy stays truthful.
			swr = stampMintedNkey(swr, natsConn.NatsUserNkey)

			// The agent below receives (and will use) the freshly minted
			// credential regardless of whether this store succeeds, so a
			// failure here leaves the KV record holding the PREVIOUS nkey
			// while the live credential is the new one — a future fencing
			// revocation keyed off the stored record would revoke the
			// wrong identity.
			//
			// One retry on conflict: the mint above is not instant, so a
			// writer can still slip in between the re-read and this store.
			// Re-read and re-stamp the newest record rather than retrying
			// with the definition we already know is stale.
			if err := n.state.StoreWorkload(workloadID, swr, revision); errors.Is(err, models.ErrStateConflict) {
				newest, newestRevision, rerr := n.state.GetWorkloadRecord(registrationRequest.RegisterType, workloadID)
				switch {
				case rerr != nil:
					// The only definition in hand is the one that just LOST
					// the CAS, so it is known-stale: a writer demonstrably
					// replaced it. Resuming from it would start the reverted
					// definition -- the silent revert this whole path exists
					// to prevent, reached through the error branch instead
					// of the happy one. Drop the workload and let the next
					// registration resume it from whatever is really on
					// file.
					n.logger.Error("failed to re-read workload record after conflict, workload dropped from resume state", slog.String("err", rerr.Error()), slog.String("workload_id", workloadID))
					continue
				case newest == nil:
					n.logger.Warn("workload record removed during resume; not resuming", slog.String("workload_id", workloadID))
					continue
				default:
					swr = stampMintedNkey(*newest, natsConn.NatsUserNkey)
					if err := n.state.StoreWorkload(workloadID, swr, newestRevision); err != nil {
						// Second conflict: give up on the stamp rather
						// than loop. The workload is still resumed, from
						// the newest definition read above, and the
						// credential the agent gets is the fresh one — only
						// the KV copy of the nkey is behind.
						n.logger.Error("failed to persist re-minted workload nkey; stored nkey is stale — credential fencing against this record would revoke the wrong key", slog.String("err", err.Error()), slog.String("workload_id", workloadID))
					}
				}
			} else if err != nil {
				n.logger.Error("failed to persist re-minted workload nkey; stored nkey is stale — credential fencing against this record would revoke the wrong key", slog.String("err", err.Error()), slog.String("workload_id", workloadID))
			}

			aswr := models.AgentStartWorkloadRequest{
				Request:       swr,
				WorkloadCreds: *natsConn,
			}
			state[workloadID] = aswr
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
		n.logger.Info("agent registered", slog.String("name", registrationRequest.Name), slog.String("type", registrationRequest.RegisterType), slog.String("agent_id", agentID))
	}
}

func (n *NexNode) handleRegisterRemoteAgent() func(micro.Request) {
	return func(r micro.Request) {
		req := new(models.RegisterRemoteAgentRequest)
		err := json.Unmarshal(r.Data(), req)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeBadRequest, "failed to unmarshal register remote agent request")
			return
		}

		err = n.aregistrar.RegisterRemoteInit(r.Headers(), req)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeForbidden, "failed agent registrar check")
			return
		}

		agentID := n.idgen.Generate(nil)
		pubNodeKey, err := n.nodeKeypair.PublicKey()
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to get public key from keypair")
			return
		}

		connData, err := n.handlerMinter.MintRegister(agentID, pubNodeKey)
		if err != nil {
			n.handlerError(r, err, models.ErrCodeInternalServerError, "failed to mint register")
			return
		}

		ret := models.RegisterRemoteAgentResponse{
			AssignedAgentId:   agentID,
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

func (n *NexNode) handleGetAgentIDByName() func(micro.Request) {
	return func(r micro.Request) {
		agentName := string(r.Data())
		if agentName == "" {
			n.handlerError(r, errors.New("agent name is required"), models.ErrCodeBadRequest, "agent name is required")
			return
		}

		if agent, err := n.registeredAgents.GetByRegisterName(agentName); err == nil {
			err = r.Respond([]byte(agent.ID))
			if err != nil {
				n.logger.Error("failed to respond to get agent id by name request", slog.String("err", err.Error()))
				return
			}
		}
	}
}

func (n *NexNode) handlerError(r micro.Request, err error, code, msg string) {
	errorID := n.loggerID.Next()

	if msg != "" {
		n.logger.Error(msg, slog.String("err", err.Error()), slog.String("error_id", errorID))
	}

	// For 5xx errors, return a generic message to avoid leaking internal details.
	// The full error is logged above with the error_id for correlation.
	clientError := err.Error()
	if strings.HasPrefix(code, "5") {
		clientError = "internal server error"
	}

	errMsg := struct {
		ErrorID string `json:"error_id"`
		Error   string `json:"error"`
	}{
		ErrorID: errorID,
		Error:   clientError,
	}

	errMsgB, marshalErr := json.Marshal(errMsg)
	if marshalErr != nil {
		n.logger.Error("failed to marshal error message", slog.String("err", marshalErr.Error()), slog.String("error_id", errorID))
		errMsgB = []byte(`{}`)
	}

	sendErr := r.Error(code, msg, errMsgB)
	if sendErr != nil {
		n.logger.Error("failed to send micro request error message", slog.String("err", sendErr.Error()), slog.String("error_id", errorID))
	}
}
