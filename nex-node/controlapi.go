package nexnode

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	controlapi "github.com/ConnectEverything/nex/control-api"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/sirupsen/logrus"
)

type ApiListener struct {
	mgr          *MachineManager
	nc           *nats.Conn
	log          *logrus.Logger
	payloadCache *payloadCache
	nodeId       string
	start        time.Time
	xk           nkeys.KeyPair
	tags         map[string]string
}

func NewApiListener(nc *nats.Conn, log *logrus.Logger, mgr *MachineManager, tags map[string]string) *ApiListener {
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

	dname, err := os.MkdirTemp("", "nexnode")
	if err != nil {
		log.WithError(err).Error("Failed to create temp directory for workload cache")
		return nil
	}

	return &ApiListener{
		mgr:          mgr,
		nc:           nc,
		log:          log,
		nodeId:       pub,
		xk:           kp,
		payloadCache: NewPayloadCache(nc, log, dname),
		start:        time.Now().UTC(),
		tags:         tags,
	}
}

func (api *ApiListener) PublicKey() string {
	pub, _ := api.mgr.kp.PublicKey()
	return pub
}

func (api *ApiListener) Start() error {
	api.nc.Subscribe(controlapi.APIPrefix+".PING", handlePing(api))
	api.nc.Subscribe(controlapi.APIPrefix+".PING."+api.nodeId, handlePing(api))
	api.nc.Subscribe(controlapi.APIPrefix+".INFO."+api.nodeId, handleInfo(api))
	api.nc.Subscribe(controlapi.APIPrefix+".RUN."+api.nodeId, handleRun(api))
	api.log.WithField("id", api.nodeId).WithField("version", VERSION).Info("NATS execution engine awaiting commands")
	return nil
}

func handleRun(api *ApiListener) func(m *nats.Msg) {
	return func(m *nats.Msg) {
		var request controlapi.RunRequest
		err := json.Unmarshal(m.Data, &request)
		if err != nil {
			api.log.WithError(err).Error("Failed to deserialize run request")
			respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Unable to deserialize run request: %s", err))
		}

		// If this passes, `DecodedClaims` contains values and WorkloadEnvironment is decrypted
		decodedClaims, err := request.Validate(api.xk)
		if err != nil {
			api.log.WithError(err).Error("Invalid run request")
			respondFail(controlapi.RunResponseType, m, fmt.Sprintf("Invalid run request: %s", err))
			return
		}
		fmt.Printf("POST VALIDATION: %+v\n", request)
		request.DecodedClaims = *decodedClaims

		// TODO: once we support another location, change this to "GetPayload"
		payloadFile, err := api.payloadCache.GetPayloadFromBucket(&request)
		if err != nil {
			api.log.WithError(err).Error("Failed to read payload from specified location")
			return
		}

		runningVm, err := api.mgr.TakeFromPool()
		if err != nil {
			api.log.WithError(err).Error("Failed to get warm VM from pool")
			return
		}

		// NOTE this makes a copy of request
		_, err = runningVm.agentClient.PostWorkload(payloadFile.Name(), request.WorkloadEnvironment)
		if err != nil {
			return
		}
		runningVm.workloadStarted = time.Now().UTC()
		runningVm.workloadSpecification = request

		if err != nil {
			api.log.WithError(err).Error("Failed to start workload in VM")
		}
		fmt.Printf("NEW workload spec claims %+v\n", runningVm.workloadSpecification.DecodedClaims)

		res := controlapi.NewEnvelope(controlapi.RunResponseType, controlapi.RunResponse{
			Started:   true,
			Name:      runningVm.workloadSpecification.DecodedClaims.Name,
			Issuer:    runningVm.workloadSpecification.DecodedClaims.Issuer,
			MachineId: runningVm.vmmID,
		}, nil)
		raw, err := json.Marshal(res)
		if err != nil {
			api.log.WithError(err).Error("Failed to marshal ping response")
		} else {
			m.Respond(raw)
		}
		api.mgr.allVms[runningVm.vmmID] = *runningVm
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
		pubX, _ := api.xk.PublicKey()
		now := time.Now().UTC()
		res := controlapi.NewEnvelope(controlapi.InfoResponseType, controlapi.InfoResponse{
			Version:    VERSION,
			PublicXKey: pubX,
			Uptime:     myUptime(now.Sub(api.start)),
			Tags:       api.tags,
			Machines:   summarizeMachines(&api.mgr.allVms),
		}, nil)
		raw, err := json.Marshal(res)
		if err != nil {
			api.log.WithError(err).Error("Failed to marshal ping response")
		} else {
			m.Respond(raw)
		}
	}

}

func summarizeMachines(vms *map[string]runningFirecracker) []controlapi.MachineSummary {
	machines := make([]controlapi.MachineSummary, 0)
	now := time.Now().UTC()
	for _, v := range *vms {
		fmt.Printf("VM: %+v\n", v)
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
	return machines
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
