package nexnode

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/pkg/errors"
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// The API listener is the command and control interface for the node server
type ApiListener struct {
	node  *Node
	mgr   *WorkloadManager
	log   *slog.Logger
	start time.Time
	xk    nkeys.KeyPair

	subz []*nats.Subscription
}

// FIXME-- stop passing node here
func NewApiListener(log *slog.Logger, mgr *WorkloadManager, node *Node) *ApiListener {
	config := node.config

	efftags := config.Tags
	efftags[controlapi.TagOS] = runtime.GOOS
	efftags[controlapi.TagArch] = runtime.GOARCH
	efftags[controlapi.TagCPUs] = strconv.FormatInt(int64(runtime.NumCPU()), 10)
	if node.config.NoSandbox {
		efftags[controlapi.TagUnsafe] = "true"
	}

	kp, err := nkeys.CreateCurveKeys()
	if err != nil {
		log.Error("Failed to create x509 curve key", slog.Any("err", err))
		return nil
	}
	xkPub, err := kp.PublicKey()
	if err != nil {
		log.Error("Failed to get public key from x509 curve key", slog.Any("err", err))
		return nil
	}

	log.Info("Use this key as the recipient for encrypted run requests", slog.String("public_xkey", xkPub))

	return &ApiListener{
		mgr:   mgr,
		log:   log,
		xk:    kp,
		start: time.Now().UTC(),
		node:  node,
		subz:  make([]*nats.Subscription, 0),
	}
}

func (api *ApiListener) Drain() error {
	for _, sub := range api.subz {
		err := sub.Drain()
		if err != nil {
			api.log.Warn("failed to drain subscription associated with api listener",
				slog.String("subject", sub.Subject),
				slog.String("error", err.Error()),
			)

			// no-op for now, try the next one...
		}

		api.log.Debug("drained subscription associated with api listener",
			slog.String("subject", sub.Subject),
		)
	}

	return nil
}

func (api *ApiListener) PublicKey() string {
	return api.node.publicKey
}

func (api *ApiListener) PublicXKey() string {
	pk, _ := api.xk.PublicKey()
	return pk
}

func (api *ApiListener) Start() error {
	var sub *nats.Subscription
	var err error

	sub, err = api.node.nc.Subscribe(controlapi.APIPrefix+".AUCTION", api.injectSpan(api.handleAuction))
	if err != nil {
		api.log.Error("Failed to subscribe to auction subject", slog.Any("err", err), slog.String("id", api.PublicKey()))
	}
	api.subz = append(api.subz, sub)

	sub, err = api.node.nc.Subscribe(controlapi.APIPrefix+".PING", api.handlePing)
	if err != nil {
		api.log.Error("Failed to subscribe to ping subject", slog.Any("err", err), slog.String("id", api.PublicKey()))
	}
	api.subz = append(api.subz, sub)

	sub, err = api.node.nc.Subscribe(controlapi.APIPrefix+".PING."+api.PublicKey(), api.handlePing)
	if err != nil {
		api.log.Error("Failed to subscribe to node-specific ping subject", slog.Any("err", err), slog.String("id", api.PublicKey()))
	}
	api.subz = append(api.subz, sub)

	sub, err = api.node.nc.Subscribe(controlapi.APIPrefix+".WPING.>", api.injectSpan(api.handleWorkloadPing))
	if err != nil {
		api.log.Error("Failed to subscribe to workload ping subject", slog.Any("err", err), slog.String("id", api.PublicKey()))
	}
	api.subz = append(api.subz, sub)

	// Namespaced subscriptions, the * below is for the namespace
	sub, err = api.node.nc.Subscribe(controlapi.APIPrefix+".INFO.*."+api.PublicKey(), api.injectSpan(api.handleInfo))
	if err != nil {
		api.log.Error("Failed to subscribe to info subject", slog.Any("err", err), slog.String("id", api.PublicKey()))
	}
	api.subz = append(api.subz, sub)

	sub, err = api.node.nc.Subscribe(controlapi.APIPrefix+".DEPLOY.*."+api.PublicKey(), api.injectSpan(api.handleDeploy))
	if err != nil {
		api.log.Error("Failed to subscribe to run subject", slog.Any("err", err), slog.String("id", api.PublicKey()))
	}
	api.subz = append(api.subz, sub)

	// FIXME? per contract, this should probably be renamed from STOP to UNDEPLOY
	sub, err = api.node.nc.Subscribe(controlapi.APIPrefix+".STOP.*."+api.PublicKey(), api.injectSpan(api.handleStop))
	if err != nil {
		api.log.Error("Failed to subscribe to stop subject", slog.Any("err", err), slog.String("id", api.PublicKey()))
	}
	api.subz = append(api.subz, sub)

	sub, err = api.node.nc.Subscribe(controlapi.APIPrefix+".LAMEDUCK."+api.PublicKey(), api.injectSpan(api.handleLameDuck))
	if err != nil {
		api.log.Error("Failed to subscribe to lame duck subject", slog.Any("error", err), slog.String("id", api.PublicKey()))
	}
	api.subz = append(api.subz, sub)

	api.log.Info("NATS execution engine awaiting commands", slog.String("id", api.PublicKey()), slog.String("version", VERSION))
	return nil
}

type apiHandler func(ctx context.Context, span trace.Span, m *nats.Msg)

func (api *ApiListener) injectSpan(h apiHandler) nats.MsgHandler {

	return func(m *nats.Msg) {
		tracer := api.node.telemetry.Tracer
		ctx := context.Background()
		_, span := tracer.Start(ctx, "Control API Request",
			trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		h(ctx, span, m)
	}

}

func (api *ApiListener) handleAuction(ctx context.Context, span trace.Span, m *nats.Msg) {
	span.SetName("Handle Auction Request")

	now := time.Now().UTC()

	filter := false

	var req *controlapi.AuctionRequest
	err := json.Unmarshal(m.Data, &req)
	if err == nil {
		// PING request was successfully parsed
		if req.Arch != nil && !strings.EqualFold(api.node.config.Tags[controlapi.TagArch], *req.Arch) {
			filter = true
		}

		if req.OS != nil && !strings.EqualFold(api.node.config.Tags[controlapi.TagOS], *req.OS) {
			filter = true
		}

		if req.Sandboxed != nil && api.node.config.NoSandbox != !*req.Sandboxed {
			filter = true
		}

		for tag := range req.Tags {
			val, ok := api.node.config.Tags[tag]
			if !ok {
				filter = true
			} else if !strings.EqualFold(val, req.Tags[tag]) {
				filter = true
			}
		}

		for _, workloadType := range req.WorkloadTypes {
			if !slices.Contains(api.node.config.WorkloadTypes, workloadType) {
				filter = true
			}
		}
	}

	if filter {
		api.log.Debug("Node not viable for deploy request specified at auction")
		span.SetStatus(codes.Ok, "Node did not match auction parameters")
		return
	}

	machines, err := api.mgr.RunningWorkloads()
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to query running machines", slog.Any("error", err))
		respondFail(controlapi.AuctionResponseType, m, "Failed to query running machines on node")
		return
	}

	res := controlapi.NewEnvelope(controlapi.AuctionResponseType, controlapi.AuctionResponse{
		NodeId:          api.PublicKey(),
		Nexus:           api.node.nexus,
		Version:         Version(),
		TargetXkey:      api.PublicXKey(),
		Uptime:          myUptime(now.Sub(api.start)),
		RunningMachines: len(machines),
		Tags:            api.node.config.Tags,
	}, nil)

	raw, err := json.Marshal(res)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to marshal ping response", slog.Any("err", err))
	} else {
		span.SetStatus(codes.Ok, "Responded to auction")
		_ = m.Respond(raw)
	}
}

func (api *ApiListener) handleDeploy(ctx context.Context, span trace.Span, m *nats.Msg) {
	span.SetName("Handle Deploy Request")

	namespace, err := extractNamespace(m.Subject)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Invalid subject for workload deployment", slog.Any("err", err))
		respondFail(controlapi.RunResponseType, m, "Invalid subject for workload deployment")
		return
	}
	span.SetAttributes(attribute.String("namespace", namespace))

	if api.node.IsLameDuck() {
		span.SetStatus(codes.Error, "Node is in lame duck mode, rejected deploy request")
		respondFail(controlapi.RunResponseType, m, "Node is in lame duck mode. Workload deploy request denied")
		return
	}

	if api.exceedsMaxWorkloadCount() {
		span.SetStatus(codes.Error, "Node is at maximum workload limit, rejected deploy request")
		respondFail(controlapi.RunResponseType, m, "Node is at maximum workload limit. Deploy request denied")
		return
	}

	var request controlapi.DeployRequest
	err = json.Unmarshal(m.Data, &request)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to deserialize deploy request", slog.Any("err", err))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Unable to deserialize deploy request: %s", err))
		return
	}

	if !slices.Contains(api.node.config.WorkloadTypes, request.WorkloadType) {
		span.SetStatus(codes.Error, "Unsupported workload type")
		api.log.Error("This node does not support the given workload type", slog.String("workload_type", string(request.WorkloadType)))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Unsupported workload type on this node: %s", string(request.WorkloadType)))
		return
	}

	if len(request.TriggerSubjects) > 0 && (request.WorkloadType != controlapi.NexWorkloadV8 &&
		request.WorkloadType != controlapi.NexWorkloadWasm) { // FIXME -- workload type comparison
		span.SetStatus(codes.Error, "Unsupported workload type for trigger subjects")
		api.log.Error("Workload type does not support trigger subject registration", slog.String("trigger_subjects", string(request.WorkloadType)))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Unsupported workload type for trigger subject registration: %s", string(request.WorkloadType)))
		return
	}
	span.SetAttributes(attribute.String("trigger_subjects", strings.Join(request.TriggerSubjects, ",")))

	if len(request.TriggerSubjects) > 0 && len(api.node.config.DenyTriggerSubjects) > 0 {
		for _, subject := range request.TriggerSubjects {
			if inDenyList(subject, api.node.config.DenyTriggerSubjects) {
				span.SetStatus(codes.Error, "Trigger subject space overlaps with blocked subjects")
				respondFail(controlapi.RunResponseType, m,
					fmt.Sprintf("The trigger subject %s overlaps with subject(s) in this node's deny list", subject))
				return
			}
		}
	}

	err = request.DecryptRequestEnvironment(api.xk)
	if err != nil {
		publicKey, _ := api.xk.PublicKey()
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to decrypt environment for deploy request", slog.String("public_key", publicKey), slog.Any("err", err))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Failed to decrypt environment for deploy request: %s", err))
		return
	}
	span.SetAttributes(attribute.String("workload_name", request.DecodedClaims.Subject))

	decodedClaims, err := request.Validate()
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Invalid deploy request", slog.Any("err", err))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Invalid deploy request: %s", err))
		return
	}

	request.DecodedClaims = *decodedClaims
	if !validateIssuer(request.DecodedClaims.Issuer, api.node.config.ValidIssuers) {
		err := fmt.Errorf("invalid workload issuer: %s", request.DecodedClaims.Issuer)
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Workload validation failed", slog.Any("err", err))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("%s", err))
		return
	}

	var agentClient *agentapi.AgentClient

	retry := 0
	for agentClient == nil {
		agentClient, err = api.mgr.SelectRandomAgent()
		if err != nil {
			api.log.Warn("Failed to resolve agent for attempted deploy",
				slog.String("error", err.Error()),
			)

			retry += 1
			if retry > agentPoolRetryMax-1 {
				api.log.Error("Exceeded warm agent retrieval retry count during attempted deploy",
					slog.Int("allowed_retries", agentPoolRetryMax),
				)

				span.SetStatus(codes.Error, err.Error())
				api.log.Error("Failed to get agent client from pool", slog.Any("err", err))
				respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Failed to get agent client from pool: %s", err))
				return
			}

			time.Sleep(time.Millisecond * 100)
		}
	}

	workloadID := agentClient.ID()
	span.SetAttributes(attribute.String("workload_id", workloadID))

	api.log.Debug("Resolved warm agent for attempted deploy",
		slog.String("workload_id", workloadID),
	)

	span.AddEvent("Started workload download")
	numBytes, workloadHash, err := api.mgr.CacheWorkload(workloadID, &request)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to cache workload bytes", slog.Any("err", err))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Failed to cache workload bytes: %s", err))
		return
	}
	span.AddEvent("Completed workload download")

	if api.exceedsMaxWorkloadSize(numBytes) {
		span.SetStatus(codes.Error, "Workload file size is too big")
		respondFail(controlapi.RunResponseType, m, "Workload file size would exceed node limitations")
		return
	}

	if api.exceedsPerNodeWorkloadSizeMax(numBytes) {
		span.SetStatus(codes.Error, "Workload file size exceeds node limitations")
		respondFail(controlapi.RunResponseType, m, "Workload file size would exceed node total limitations")
		return
	}

	agentDeployRequest := agentDeployRequestFromControlDeployRequest(&request, namespace, numBytes, *workloadHash)

	api.log.
		Info("Submitting workload to agent",
			slog.String("namespace", namespace),
			slog.String("workload", *agentDeployRequest.WorkloadName),
			slog.String("workload_id", workloadID),
			slog.Uint64("workload_size", numBytes),
			slog.String("workload_sha256", *workloadHash),
			slog.String("type", string(request.WorkloadType)),
		)

	span.AddEvent("Created agent deploy request")
	err = api.mgr.DeployWorkload(agentClient, agentDeployRequest)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to deploy workload",
			slog.String("error", err.Error()),
		)
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Failed to deploy workload: %s", err))
		return
	}
	span.AddEvent("Agent deploy request accepted")

	if _, ok := api.mgr.handshakes[workloadID]; !ok {
		span.SetStatus(codes.Error, "Tried to deploy into non-handshaked agent")
		api.log.Error("Attempted to deploy workload into bad process (no handshake)",
			slog.String("workload_id", workloadID),
		)
		respondFail(controlapi.RunResponseType, m, "Could not deploy workload, agent pool did not initialize properly")
		return
	}

	workloadName := request.DecodedClaims.Subject
	span.SetAttributes(attribute.String("workload_name", workloadName))

	api.log.Info("Workload deployed", slog.String("workload", workloadName), slog.String("workload_id", workloadID))

	res := controlapi.NewEnvelope(controlapi.RunResponseType, controlapi.RunResponse{
		Started: true,
		Name:    workloadName,
		Issuer:  request.DecodedClaims.Issuer,
		ID:      workloadID, // FIXME-- rename to match
	}, nil)

	raw, err := json.Marshal(res)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to marshal deploy response", slog.Any("err", err))
	} else {
		span.SetStatus(codes.Ok, "Workload deployed")
		_ = m.Respond(raw)
	}
}

func (api *ApiListener) handlePing(m *nats.Msg) {
	now := time.Now().UTC()

	machines, err := api.mgr.RunningWorkloads()
	if err != nil {
		api.log.Error("Failed to query running machines", slog.Any("error", err))
		respondFail(controlapi.PingResponseType, m, "Failed to query running machines on node")
		return
	}

	res := controlapi.NewEnvelope(controlapi.PingResponseType, controlapi.PingResponse{
		NodeId:          api.PublicKey(),
		Nexus:           api.node.nexus,
		Version:         Version(),
		TargetXkey:      api.PublicXKey(),
		Uptime:          myUptime(now.Sub(api.start)),
		RunningMachines: len(machines),
		Tags:            api.node.config.Tags,
	}, nil)

	raw, err := json.Marshal(res)
	if err != nil {
		api.log.Error("Failed to marshal ping response", slog.Any("err", err))
	} else {
		_ = m.Respond(raw)
	}
}

func (api *ApiListener) handleStop(ctx context.Context, span trace.Span, m *nats.Msg) {
	span.SetName("Handle workload stop request")
	namespace, err := extractNamespace(m.Subject)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Invalid subject for workload stop", slog.Any("err", err))
		respondFail(controlapi.StopResponseType, m, "Invalid subject for workload stop")
		return
	}

	var request controlapi.StopRequest
	err = json.Unmarshal(m.Data, &request)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to deserialize stop request", slog.Any("err", err))
		respondFail(controlapi.StopResponseType, m, fmt.Sprintf("Unable to deserialize stop request: %s", err))
		return
	}

	deployRequest, err := api.mgr.LookupWorkload(request.WorkloadId)
	if err != nil {
		span.SetStatus(codes.Error, "No such workload")
		api.log.Error("Stop request: no such workload", slog.String("workload_id", request.WorkloadId))
		respondFail(controlapi.StopResponseType, m, "No such workload")
		return
	}

	err = request.Validate(&deployRequest.DecodedClaims)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to validate stop request", slog.Any("err", err))
		respondFail(controlapi.StopResponseType, m, fmt.Sprintf("Invalid stop request: %s", err))
		return
	}

	if *deployRequest.Namespace != namespace {
		api.log.Error("Namespace mismatch on workload stop request",
			slog.String("namespace", *deployRequest.Namespace),
			slog.String("targetnamespace", namespace),
		)

		span.SetStatus(codes.Error, "Namespace mismatch")
		respondFail(controlapi.StopResponseType, m, "No such workload") // do not expose ID existence to avoid existence probes
		return
	}

	err = api.mgr.StopWorkload(request.WorkloadId, true)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to stop workload", slog.Any("err", err))
		respondFail(controlapi.StopResponseType, m, fmt.Sprintf("Failed to stop workload: %s", err))
	}

	res := controlapi.NewEnvelope(controlapi.StopResponseType, controlapi.StopResponse{
		Stopped: true,
		Name:    deployRequest.DecodedClaims.Subject,
		Issuer:  deployRequest.DecodedClaims.Issuer,
		ID:      request.WorkloadId,
	}, nil)
	raw, err := json.Marshal(res)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to marshal run response", slog.Any("err", err))
	} else {
		span.SetStatus(codes.Ok, "Responded to stop request")
		_ = m.Respond(raw)
	}
}

// $NEX.WPING.{namespace}.{workloadId}
func (api *ApiListener) handleWorkloadPing(ctx context.Context, span trace.Span, m *nats.Msg) {
	span.SetName("Handle workload ping")
	// Note that this ping _only_ responds on success, all others are silent
	// $NEX.WPING.{namespace}.{workloadId}
	// result payload looks exactly like a node ping reply
	tokens := strings.Split(m.Subject, ".")

	namespace := ""
	workloadId := ""

	if len(tokens) > 2 {
		namespace = tokens[2]
	}
	if len(tokens) > 3 {
		workloadId = tokens[3]
	}

	machines, err := api.mgr.RunningWorkloads()
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to query running machines", slog.Any("error", err))
		return
	}

	summaries := summarizeMachinesForPing(machines, namespace, workloadId)
	if len(summaries) > 0 {
		now := time.Now().UTC()
		res := controlapi.NewEnvelope(controlapi.PingResponseType, controlapi.WorkloadPingResponse{
			NodeId:          api.PublicKey(),
			TargetXkey:      api.PublicXKey(),
			Version:         Version(),
			Uptime:          myUptime(now.Sub(api.start)),
			RunningMachines: summaries,
			Tags:            api.node.config.Tags,
		}, nil)

		raw, err := json.Marshal(res)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			api.log.Error("Failed to marshal ping response", slog.Any("err", err))
		} else {
			span.SetStatus(codes.Ok, "Returned workload ping")
			_ = m.Respond(raw)
		}
	}
	span.SetStatus(codes.Ok, "No machines matched ping request")

	// silence if there were no matching machines
}

func (api *ApiListener) handleLameDuck(ctx context.Context, span trace.Span, m *nats.Msg) {
	span.SetName("Handle enter lameduck")
	err := api.node.EnterLameDuck()
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to enter lame duck mode", slog.Any("error", err))
		respondFail(controlapi.LameDuckResponseType, m, "Failed to enter lame duck mode")
		return
	}
	res := controlapi.NewEnvelope(controlapi.LameDuckResponseType, controlapi.LameDuckResponse{
		Success: true,
		NodeId:  api.PublicKey(),
	}, nil)
	raw, err := json.Marshal(res)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to serialize response", slog.Any("error", err))
		respondFail(controlapi.LameDuckResponseType, m, "Serialization failure")
	} else {
		span.SetStatus(codes.Ok, "Responded to lame duck request")
		_ = m.Respond(raw)
	}
}

func (api *ApiListener) handleInfo(ctx context.Context, span trace.Span, m *nats.Msg) {
	span.SetName("Handle info request")
	namespace, err := extractNamespace(m.Subject)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to extract namespace for info request", slog.Any("err", err))
		respondFail(controlapi.InfoResponseType, m, "Failed to extract namespace for info request")
		return
	}

	workloads, err := api.mgr.RunningWorkloads()
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to query running workloads", slog.Any("error", err))
		respondFail(controlapi.InfoResponseType, m, "Failed to query running workloads on node")
		return
	}

	namespaceWorkloads := make([]controlapi.MachineSummary, 0)
	for _, w := range workloads {
		if strings.EqualFold(w.Namespace, namespace) {
			namespaceWorkloads = append(namespaceWorkloads, w)
		}
	}

	pubX, err := api.xk.PublicKey()
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to query running workloads", slog.Any("error", err))
		respondFail(controlapi.InfoResponseType, m, "Failed to query running workloads on node")
		return
	}

	stats, err := ReadMemoryStats()
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to query running workloads", slog.Any("error", err))
		respondFail(controlapi.InfoResponseType, m, "Failed to query running workloads on node")
		return
	}

	res := controlapi.NewEnvelope(controlapi.InfoResponseType, controlapi.InfoResponse{
		AvailableAgents:        len(api.mgr.poolAgents),
		Machines:               namespaceWorkloads,
		Memory:                 stats,
		PublicXKey:             pubX,
		SupportedWorkloadTypes: api.node.config.WorkloadTypes,
		Tags:                   api.node.config.Tags,
		Uptime:                 myUptime(time.Now().UTC().Sub(api.start)),
		Version:                VERSION,
	}, nil)

	raw, err := json.Marshal(res)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		api.log.Error("Failed to marshal info response", slog.Any("err", err))
	} else {
		span.SetStatus(codes.Ok, "Responded to info response")
		_ = m.Respond(raw)
	}
}

func (api *ApiListener) exceedsMaxWorkloadCount() bool {
	return api.node.config.NodeLimits.MaxWorkloads > 0 &&
		len(api.mgr.liveAgents) >= api.node.config.NodeLimits.MaxWorkloads
}

func (api *ApiListener) exceedsMaxWorkloadSize(bytes uint64) bool {
	return api.node.config.NodeLimits.MaxWorkloadBytes > 0 &&
		bytes > uint64(api.node.config.NodeLimits.MaxWorkloadBytes)
}

func (api *ApiListener) exceedsPerNodeWorkloadSizeMax(bytes uint64) bool {
	return api.node.config.NodeLimits.MaxTotalBytes > 0 &&
		bytes+api.mgr.TotalRunningWorkloadBytes() > uint64(api.node.config.NodeLimits.MaxTotalBytes)
}

func summarizeMachinesForPing(workloads []controlapi.MachineSummary, namespace string, workloadId string) []controlapi.WorkloadPingMachineSummary {
	machines := make([]controlapi.WorkloadPingMachineSummary, 0)
	for _, w := range workloads {
		if matchesSearch(w, namespace, workloadId) {
			reply := controlapi.WorkloadPingMachineSummary{
				Id:           w.Id,
				Namespace:    w.Namespace, // return the real namespace rather than the search criteria, which could be ""
				Name:         w.Workload.Name,
				WorkloadType: w.Workload.WorkloadType,
			}
			machines = append(machines, reply)
		}
	}
	return machines
}

func matchesSearch(w controlapi.MachineSummary, namespace string, workloadId string) bool {
	namespace = strings.TrimSpace(namespace)
	workloadId = strings.TrimSpace(workloadId)

	if namespace != "" {
		if !strings.EqualFold(w.Namespace, namespace) {
			return false
		}
	}
	if workloadId != "" {
		if w.Id != workloadId &&
			!strings.EqualFold(w.Workload.Name, workloadId) {
			return false
		}
	}

	return true
}

func validateIssuer(issuer string, validIssuers []string) bool {
	if len(validIssuers) == 0 {
		return true
	}
	return slices.Contains(validIssuers, issuer)
}

// This is the same uptime code as the NATS server, for consistency
func myUptime(d time.Duration) string {
	// Just use total seconds for uptime, and display days / years
	tsecs := d / time.Second
	tmins := tsecs / 60
	thrs := tmins / 60
	tdays := thrs / 24
	tyrs := tdays / 365

	if tyrs > 0 {
		return fmt.Sprintf("%dy%dd%dh%dm%ds", tyrs, tdays%365, thrs%24, tmins%60, tsecs%60)
	}
	if tdays > 0 {
		return fmt.Sprintf("%dd%dh%dm%ds", tdays, thrs%24, tmins%60, tsecs%60)
	}
	if thrs > 0 {
		return fmt.Sprintf("%dh%dm%ds", thrs, tmins%60, tsecs%60)
	}
	if tmins > 0 {
		return fmt.Sprintf("%dm%ds", tmins, tsecs%60)
	}
	return fmt.Sprintf("%ds", tsecs)
}

func inDenyList(subject string, denyList []string) bool {
	for _, target := range denyList {
		if server.SubjectsCollide(subject, target) {
			return true
		}
	}
	return false
}

func respondFail(responseType string, m *nats.Msg, reason string) {
	env := controlapi.NewEnvelope(responseType, []byte{}, &reason)
	jenv, _ := json.Marshal(env)
	_ = m.Respond(jenv)
}

func extractNamespace(subject string) (string, error) {
	tokens := strings.Split(subject, ".")
	// we need at least $NEX.{op}.{namespace}
	if len(tokens) < 3 {
		// this shouldn't ever happen if our subscriptions are defined properly
		return "", errors.Errorf("Invalid subject - could not detect a namespace")
	}
	return tokens[2], nil
}
