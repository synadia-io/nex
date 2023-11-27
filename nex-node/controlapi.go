package nexnode

import (
	"encoding/json"
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	controlapi "github.com/ConnectEverything/nex/control-api"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// The API listener is the command and control interface for the node server
type ApiListener struct {
	mgr    *MachineManager
	log    *logrus.Logger
	nodeId string
	start  time.Time
	xk     nkeys.KeyPair
	tags   map[string]string
}

func NewApiListener(log *logrus.Logger, mgr *MachineManager, tags map[string]string) *ApiListener {
	pub, _ := mgr.kp.PublicKey()
	efftags := tags
	efftags[controlapi.TagOS] = runtime.GOOS
	efftags[controlapi.TagArch] = runtime.GOARCH
	efftags[controlapi.TagCPUs] = strconv.FormatInt(int64(runtime.NumCPU()), 10)

	kp, err := nkeys.CreateCurveKeys()
	if err != nil {
		log.WithError(err).Error("Failed to create x509 curve key!")
		return nil
	}
	xkPub, err := kp.PublicKey()
	if err != nil {
		log.WithError(err).Error("Failed to get public key from x509 curve key!")
		return nil
	}

	log.WithField("public_xkey", xkPub).Info("Use this key as the recipient for encrypted run requests")

	return &ApiListener{
		mgr:    mgr,
		log:    log,
		nodeId: pub,
		xk:     kp,
		start:  time.Now().UTC(),
		tags:   tags,
	}
}

func (api *ApiListener) PublicKey() string {
	pub, _ := api.mgr.kp.PublicKey()
	return pub
}

func (api *ApiListener) Start() error {

	api.mgr.nc.Subscribe(controlapi.APIPrefix+".PING", handlePing(api))
	api.mgr.nc.Subscribe(controlapi.APIPrefix+".PING."+api.nodeId, handlePing(api))

	// Namespaced subscriptions, the * below is for the namespace
	api.mgr.nc.Subscribe(controlapi.APIPrefix+".INFO.*."+api.nodeId, handleInfo(api))
	api.mgr.nc.Subscribe(controlapi.APIPrefix+".RUN.*."+api.nodeId, handleRun(api))
	api.mgr.nc.Subscribe(controlapi.APIPrefix+".STOP.*."+api.nodeId, handleStop(api))

	api.log.WithField("id", api.nodeId).WithField("version", VERSION).Info("NATS execution engine awaiting commands")
	return nil
}

func handleStop(api *ApiListener) func(m *nats.Msg) {
	return func(m *nats.Msg) {
		namespace, err := extractNamespace(m.Subject)
		if err != nil {
			api.log.WithError(err).Error("Invalid subject for workload stop")
			respondFail(controlapi.StopResponseType, m, "Invalid subject for workload stop")
			return
		}
		var request controlapi.StopRequest
		err = json.Unmarshal(m.Data, &request)
		if err != nil {
			api.log.WithError(err).Error("Failed to deserialize stop request")
			respondFail(controlapi.StopResponseType, m, fmt.Sprintf("Unable to deserialize stop request: %s", err))
			return
		}

		vm := api.mgr.LookupMachine(request.WorkloadId)
		if vm == nil {
			api.log.WithField("vmid", request.WorkloadId).Error("Stop request: no such workload")
			respondFail(controlapi.StopResponseType, m, "No such workload")
			return
		}

		if vm.namespace != namespace {
			api.log.
				WithField("namespace", vm.namespace).
				WithField("targetnamespace", namespace).
				Error("Namespace mismatch on workload stop request")
			respondFail(controlapi.StopResponseType, m, "No such workload") // do not expose ID existence to avoid existence probes
			return
		}

		err = request.Validate(&vm.workloadSpecification.DecodedClaims)
		if err != nil {
			api.log.WithError(err).Error("Failed to validate stop request")
			respondFail(controlapi.StopResponseType, m, fmt.Sprintf("Invalid stop request: %s", err))
			return
		}

		err = api.mgr.StopMachine(request.WorkloadId)
		if err != nil {
			api.log.WithError(err).Error("Failed to stop workload")
			respondFail(controlapi.StopResponseType, m, fmt.Sprintf("Failed to stop workload: %s", err))
		}

		res := controlapi.NewEnvelope(controlapi.StopResponseType, controlapi.StopResponse{
			Stopped:   true,
			Name:      vm.workloadSpecification.DecodedClaims.Subject,
			Issuer:    vm.workloadSpecification.DecodedClaims.Issuer,
			MachineId: vm.vmmID,
		}, nil)
		raw, err := json.Marshal(res)
		if err != nil {
			api.log.WithError(err).Error("Failed to marshal run response")
		} else {
			m.Respond(raw)
		}

	}
}

func handleRun(api *ApiListener) func(m *nats.Msg) {
	return func(m *nats.Msg) {
		namespace, err := extractNamespace(m.Subject)
		if err != nil {
			api.log.WithError(err).Error("Invalid subject for workload run")
			respondFail(controlapi.RunResponseType, m, "Invalid subject for workload run")
			return
		}
		var request controlapi.RunRequest
		err = json.Unmarshal(m.Data, &request)
		if err != nil {
			api.log.WithError(err).Error("Failed to deserialize run request")
			respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Unable to deserialize run request: %s", err))
			return
		}

		decodedClaims, err := request.Validate(api.xk)
		if err != nil {
			api.log.WithError(err).Error("Invalid run request")
			respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Invalid run request: %s", err))
			return
		}
		request.DecodedClaims = *decodedClaims
		if !validateIssuer(request.DecodedClaims.Issuer, api.mgr.config.ValidIssuers) {
			err := fmt.Errorf("invalid workload issuer: %s", request.DecodedClaims.Issuer)
			api.log.WithError(err).Error("Workload validation failed")
			respondFail(controlapi.RunResponseType, m, fmt.Sprintf("%s", err))
		}

		err = api.mgr.CacheWorkload(&request)
		if err != nil {
			api.log.WithError(err).Error("Failed to cache workload bytes")
			respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Failed to cache workload bytes: %s", err))
			return
		}

		runningVm, err := api.mgr.TakeFromPool()
		if err != nil {
			api.log.WithError(err).Error("Failed to get warm VM from pool")
			respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Failed to pull warm VM from ready pool: %s", err))
			return
		}

		workloadName := request.DecodedClaims.Subject

		api.log.
			WithField("vmid", runningVm.vmmID).
			WithField("namespace", namespace).
			WithField("workload", workloadName).
			Info("Submitting workload to VM")

		err = api.mgr.DispatchWork(runningVm, workloadName, namespace, request)

		if err != nil {
			api.log.WithError(err).Error("Failed to start workload in VM")
			respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Unable to submit workload to agent process: %s", err))
			return
		}
		api.log.WithField("workload", workloadName).WithField("vmid", runningVm.vmmID).Info("Work accepted")

		res := controlapi.NewEnvelope(controlapi.RunResponseType, controlapi.RunResponse{
			Started:   true,
			Name:      workloadName,
			Issuer:    runningVm.workloadSpecification.DecodedClaims.Issuer,
			MachineId: runningVm.vmmID,
		}, nil)
		raw, err := json.Marshal(res)
		if err != nil {
			api.log.WithError(err).Error("Failed to marshal run response")
		} else {
			m.Respond(raw)
		}
	}
}

func handlePing(api *ApiListener) func(m *nats.Msg) {
	return func(m *nats.Msg) {
		now := time.Now().UTC()
		res := controlapi.NewEnvelope(controlapi.PingResponseType, controlapi.PingResponse{
			NodeId:          api.nodeId,
			Version:         VERSION,
			Uptime:          myUptime(now.Sub(api.start)),
			RunningMachines: len(api.mgr.allVms),
			Tags:            api.tags,
		}, nil)
		raw, err := json.Marshal(res)
		if err != nil {
			api.log.WithError(err).Error("Failed to marshal ping response")
		} else {
			m.Respond(raw)
		}
	}

}

func handleInfo(api *ApiListener) func(m *nats.Msg) {
	return func(m *nats.Msg) {
		namespace, err := extractNamespace(m.Subject)
		if err != nil {
			api.log.WithError(err).Error("Failed to extract namespace for info request")
			respondFail(controlapi.InfoResponseType, m, "Failed to extract namespace for info request")
			return
		}
		pubX, _ := api.xk.PublicKey()
		now := time.Now().UTC()
		stats, _ := ReadMemoryStats()
		res := controlapi.NewEnvelope(controlapi.InfoResponseType, controlapi.InfoResponse{
			Version:    VERSION,
			PublicXKey: pubX,
			Uptime:     myUptime(now.Sub(api.start)),
			Tags:       api.tags,
			Machines:   summarizeMachines(&api.mgr.allVms, namespace),
			Memory:     stats,
		}, nil)
		raw, err := json.Marshal(res)
		if err != nil {
			api.log.WithError(err).Error("Failed to marshal ping response")
		} else {
			m.Respond(raw)
		}
	}

}

func summarizeMachines(vms *map[string]*runningFirecracker, namespace string) []controlapi.MachineSummary {
	machines := make([]controlapi.MachineSummary, 0)
	now := time.Now().UTC()
	for _, v := range *vms {
		if v.namespace == namespace {
			machine := controlapi.MachineSummary{
				Id:      v.vmmID,
				Healthy: true, // TODO cache last health status
				Uptime:  myUptime(now.Sub(v.machineStarted)),
				Workload: controlapi.WorkloadSummary{
					Name:         v.workloadSpecification.DecodedClaims.Subject,
					Description:  v.workloadSpecification.Description,
					Runtime:      myUptime(now.Sub(v.workloadStarted)),
					WorkloadType: v.workloadSpecification.WorkloadType,
					//Hash:         v.workloadSpecification.DecodedClaims.Data["hash"].(string),
				},
			}

			machines = append(machines, machine)
		}
	}
	return machines
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
	m.Respond(jenv)
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
