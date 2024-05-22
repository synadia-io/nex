package nexnode

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/pkg/errors"
	controlapi "github.com/synadia-io/nex/control-api"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"github.com/synadia-io/nex/internal/models"
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

	sub, err = api.node.nc.Subscribe(controlapi.APIPrefix+".WPING.>", api.handleWorkloadPing)
	if err != nil {
		api.log.Error("Failed to subscribe to workload ping subject", slog.Any("err", err), slog.String("id", api.PublicKey()))
	}
	api.subz = append(api.subz, sub)

	// Namespaced subscriptions, the * below is for the namespace
	sub, err = api.node.nc.Subscribe(controlapi.APIPrefix+".INFO.*."+api.PublicKey(), api.handleInfo)
	if err != nil {
		api.log.Error("Failed to subscribe to info subject", slog.Any("err", err), slog.String("id", api.PublicKey()))
	}
	api.subz = append(api.subz, sub)

	sub, err = api.node.nc.Subscribe(controlapi.APIPrefix+".DEPLOY.*."+api.PublicKey(), api.handleDeploy)
	if err != nil {
		api.log.Error("Failed to subscribe to run subject", slog.Any("err", err), slog.String("id", api.PublicKey()))
	}
	api.subz = append(api.subz, sub)

	// FIXME? per contract, this should probably be renamed from STOP to UNDEPLOY
	sub, err = api.node.nc.Subscribe(controlapi.APIPrefix+".STOP.*."+api.PublicKey(), api.handleStop)
	if err != nil {
		api.log.Error("Failed to subscribe to stop subject", slog.Any("err", err), slog.String("id", api.PublicKey()))
	}
	api.subz = append(api.subz, sub)

	sub, err = api.node.nc.Subscribe(controlapi.APIPrefix+".LAMEDUCK."+api.PublicKey(), api.handleLameDuck)
	if err != nil {
		api.log.Error("Failed to subscribe to lame duck subject", slog.Any("error", err), slog.String("id", api.PublicKey()))
	}
	api.subz = append(api.subz, sub)

	api.log.Info("NATS execution engine awaiting commands", slog.String("id", api.PublicKey()), slog.String("version", VERSION))
	return nil
}

func (api *ApiListener) handleStop(m *nats.Msg) {
	namespace, err := extractNamespace(m.Subject)
	if err != nil {
		api.log.Error("Invalid subject for workload stop", slog.Any("err", err))
		respondFail(controlapi.StopResponseType, m, "Invalid subject for workload stop")
		return
	}

	var request controlapi.StopRequest
	err = json.Unmarshal(m.Data, &request)
	if err != nil {
		api.log.Error("Failed to deserialize stop request", slog.Any("err", err))
		respondFail(controlapi.StopResponseType, m, fmt.Sprintf("Unable to deserialize stop request: %s", err))
		return
	}

	deployRequest, _ := api.mgr.LookupWorkload(request.WorkloadId)
	if deployRequest == nil {
		api.log.Error("Stop request: no such workload", slog.String("workload_id", request.WorkloadId))
		respondFail(controlapi.StopResponseType, m, "No such workload")
		return
	}

	err = request.Validate(&deployRequest.DecodedClaims)
	if err != nil {
		api.log.Error("Failed to validate stop request", slog.Any("err", err))
		respondFail(controlapi.StopResponseType, m, fmt.Sprintf("Invalid stop request: %s", err))
		return
	}

	if *deployRequest.Namespace != namespace {
		api.log.Error("Namespace mismatch on workload stop request",
			slog.String("namespace", *deployRequest.Namespace),
			slog.String("targetnamespace", namespace),
		)

		respondFail(controlapi.StopResponseType, m, "No such workload") // do not expose ID existence to avoid existence probes
		return
	}

	err = api.mgr.StopWorkload(request.WorkloadId, true)
	if err != nil {
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
		api.log.Error("Failed to marshal run response", slog.Any("err", err))
	} else {
		_ = m.Respond(raw)
	}
}

func (api *ApiListener) handleDeploy(m *nats.Msg) {
	namespace, err := extractNamespace(m.Subject)
	if err != nil {
		api.log.Error("Invalid subject for workload deployment", slog.Any("err", err))
		respondFail(controlapi.RunResponseType, m, "Invalid subject for workload deployment")
		return
	}

	if api.node.IsLameDuck() {
		respondFail(controlapi.RunResponseType, m, "Node is in lame duck mode. Workload deploy request rejected")
		return
	}

	var request controlapi.DeployRequest
	err = json.Unmarshal(m.Data, &request)
	if err != nil {
		api.log.Error("Failed to deserialize deploy request", slog.Any("err", err))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Unable to deserialize deploy request: %s", err))
		return
	}

	if !slices.Contains(api.node.config.WorkloadTypes, request.WorkloadType) {
		api.log.Error("This node does not support the given workload type", slog.String("workload_type", string(request.WorkloadType)))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Unsupported workload type on this node: %s", string(request.WorkloadType)))
		return
	}

	if len(request.TriggerSubjects) > 0 && (request.WorkloadType != models.NexExecutionProviderV8 &&
		request.WorkloadType != models.NexExecutionProviderWasm) { // FIXME -- workload type comparison
		api.log.Error("Workload type does not support trigger subject registration", slog.String("trigger_subjects", string(request.WorkloadType)))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Unsupported workload type for trigger subject registration: %s", string(request.WorkloadType)))
		return
	}

	err = request.DecryptRequestEnvironment(api.xk)
	if err != nil {
		publicKey, _ := api.xk.PublicKey()
		api.log.Error("Failed to decrypt environment for deploy request", slog.String("public_key", publicKey), slog.Any("err", err))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Failed to decrypt environment for deploy request: %s", err))
		return
	}

	decodedClaims, err := request.Validate()
	if err != nil {
		api.log.Error("Invalid deploy request", slog.Any("err", err))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Invalid deploy request: %s", err))
		return
	}

	request.DecodedClaims = *decodedClaims
	if !validateIssuer(request.DecodedClaims.Issuer, api.node.config.ValidIssuers) {
		err := fmt.Errorf("invalid workload issuer: %s", request.DecodedClaims.Issuer)
		api.log.Error("Workload validation failed", slog.Any("err", err))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("%s", err))
	}

	numBytes, workloadHash, err := api.mgr.CacheWorkload(&request)
	if err != nil {
		api.log.Error("Failed to cache workload bytes", slog.Any("err", err))
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Failed to cache workload bytes: %s", err))
		return
	}

	deployRequest := &agentapi.DeployRequest{
		Argv:                 request.Argv,
		DecodedClaims:        request.DecodedClaims,
		Description:          request.Description,
		EncryptedEnvironment: request.Environment,
		Environment:          request.WorkloadEnvironment,
		Essential:            request.Essential,
		Hash:                 *workloadHash,
		JsDomain:             request.JsDomain,
		Location:             request.Location,
		Namespace:            &namespace,
		RetryCount:           request.RetryCount,
		RetriedAt:            request.RetriedAt,
		SenderPublicKey:      request.SenderPublicKey,
		TargetNode:           request.TargetNode,
		TotalBytes:           int64(numBytes),
		TriggerSubjects:      request.TriggerSubjects,
		WorkloadName:         &request.DecodedClaims.Subject,
		WorkloadType:         request.WorkloadType, // FIXME-- audit all types for string -> *string, and validate...
		WorkloadJwt:          request.WorkloadJwt,
	}

	api.log.
		Info("Submitting workload to agent",
			slog.String("namespace", namespace),
			slog.String("workload", *deployRequest.WorkloadName),
			slog.Uint64("workload_size", numBytes),
			slog.String("workload_sha256", *workloadHash),
			slog.String("type", string(request.WorkloadType)),
		)

	workloadID, err := api.mgr.DeployWorkload(deployRequest)
	if err != nil {
		api.log.Error("Failed to deploy workload",
			slog.String("error", err.Error()),
		)
		respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Failed to deploy workload: %s", err))
		return
	}

	if _, ok := api.mgr.handshakes[*workloadID]; !ok {
		api.log.Error("Attempted to deploy workload into bad process (no handshake)",
			slog.String("workload_id", *workloadID),
		)
		respondFail(controlapi.RunResponseType, m, "Could not deploy workload, agent pool did not initialize properly")
		return
	}
	workloadName := request.DecodedClaims.Subject

	api.log.Info("Workload deployed", slog.String("workload", workloadName), slog.String("workload_id", *workloadID))

	res := controlapi.NewEnvelope(controlapi.RunResponseType, controlapi.RunResponse{
		Started: true,
		Name:    workloadName,
		Issuer:  request.DecodedClaims.Issuer,
		ID:      *workloadID, // FIXME-- rename to match
	}, nil)

	raw, err := json.Marshal(res)
	if err != nil {
		api.log.Error("Failed to marshal deploy response", slog.Any("err", err))
	} else {
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

// $NEX.WPING.{namespace}.{workloadId}
func (api *ApiListener) handleWorkloadPing(m *nats.Msg) {
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
			api.log.Error("Failed to marshal ping response", slog.Any("err", err))
		} else {
			_ = m.Respond(raw)
		}
	}

	// silence if there were no matching machines
}

func (api *ApiListener) handleLameDuck(m *nats.Msg) {
	err := api.node.EnterLameDuck()
	if err != nil {
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
		api.log.Error("Failed to serialize response", slog.Any("error", err))
		respondFail(controlapi.LameDuckResponseType, m, "Serialization failure")
	} else {
		_ = m.Respond(raw)
	}
}

func (api *ApiListener) handleInfo(m *nats.Msg) {
	namespace, err := extractNamespace(m.Subject)
	if err != nil {
		api.log.Error("Failed to extract namespace for info request", slog.Any("err", err))
		respondFail(controlapi.InfoResponseType, m, "Failed to extract namespace for info request")
		return
	}

	machines, err := api.mgr.RunningWorkloads()
	if err != nil {
		api.log.Error("Failed to query running machines", slog.Any("error", err))
		respondFail(controlapi.PingResponseType, m, "Failed to query running machines on node")
		return
	}

	pubX, _ := api.xk.PublicKey()
	now := time.Now().UTC()
	stats, _ := ReadMemoryStats()
	res := controlapi.NewEnvelope(controlapi.InfoResponseType, controlapi.InfoResponse{
		Version:                VERSION,
		PublicXKey:             pubX,
		Uptime:                 myUptime(now.Sub(api.start)),
		Tags:                   api.node.config.Tags,
		SupportedWorkloadTypes: api.node.config.WorkloadTypes,
		Machines:               summarizeMachines(machines, namespace), // filters by namespace
		Memory:                 stats,
	}, nil)

	raw, err := json.Marshal(res)
	if err != nil {
		api.log.Error("Failed to marshal ping response", slog.Any("err", err))
	} else {
		_ = m.Respond(raw)
	}
}

func summarizeMachines(workloads []controlapi.MachineSummary, namespace string) []controlapi.MachineSummary {
	machines := make([]controlapi.MachineSummary, 0)
	for _, w := range workloads {
		if strings.EqualFold(w.Namespace, namespace) {
			machines = append(machines, w)
		}
	}
	return machines
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
